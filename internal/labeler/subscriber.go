// Package labeler subscribes to external ATProto labeler streams.
package labeler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/events/schedulers/sequential"
	"github.com/gorilla/websocket"

	"github.com/GainForest/hyperindex/internal/database/repositories"
)

const (
	defaultReconnectInitial = time.Second
	defaultReconnectCeiling = 60 * time.Second
	defaultReadLimit        = 4 << 20 // 4 MiB
)

// Config configures external labeler websocket subscriptions.
type Config struct {
	URLs         []string
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	ReadLimit    int64
}

// Subscriber manages one external labeler websocket subscription per URL.
type Subscriber struct {
	repo   *repositories.ExternalLabelsRepository
	cfg    Config
	dialer *websocket.Dialer
}

// NewSubscriber creates a labeler subscriber.
func NewSubscriber(repo *repositories.ExternalLabelsRepository, cfg Config) *Subscriber {
	cfg = normalizeConfig(cfg)
	dialer := *websocket.DefaultDialer
	return &Subscriber{
		repo:   repo,
		cfg:    cfg,
		dialer: &dialer,
	}
}

// Start launches one goroutine per configured subscription URL.
func (s *Subscriber) Start(ctx context.Context) {
	for _, subscriptionURL := range s.cfg.URLs {
		subscriptionURL := subscriptionURL
		go s.runSubscription(ctx, subscriptionURL)
	}
}

func (s *Subscriber) runSubscription(ctx context.Context, subscriptionURL string) {
	backoff := s.cfg.ReconnectMin

	for {
		err := s.runOnce(ctx, subscriptionURL)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			slog.Error("Labeler subscription disconnected", "url", subscriptionURL, "error", err, "backoff", backoff)
			if updateErr := s.repo.UpdateError(ctx, subscriptionURL, err.Error()); updateErr != nil {
				slog.Warn("Failed to record labeler subscription error", "url", subscriptionURL, "error", updateErr)
			}
		} else {
			slog.Warn("Labeler subscription closed unexpectedly", "url", subscriptionURL, "backoff", backoff)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}

		backoff *= 2
		if backoff > s.cfg.ReconnectMax {
			backoff = s.cfg.ReconnectMax
		}
	}
}

func (s *Subscriber) runOnce(ctx context.Context, subscriptionURL string) error {
	state, err := s.repo.EnsureState(ctx, subscriptionURL)
	if err != nil {
		return fmt.Errorf("ensure labeler subscription state: %w", err)
	}

	streamURL, err := buildSubscribeURL(subscriptionURL, state.LastSeq)
	if err != nil {
		return fmt.Errorf("build labeler subscription URL: %w", err)
	}

	slog.Info("Connecting to labeler subscription", "url", subscriptionURL, "cursor", state.LastSeq)
	conn, resp, err := s.dialer.DialContext(ctx, streamURL, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("connect to labeler subscription: %w", err)
	}
	defer conn.Close()

	conn.SetReadLimit(s.cfg.ReadLimit)

	if err := s.repo.UpdateConnected(ctx, subscriptionURL); err != nil {
		return fmt.Errorf("record labeler connection: %w", err)
	}

	callbacks := s.callbacks(subscriptionURL)
	sched := sequential.NewScheduler("labeler:"+subscriptionURL, callbacks.EventHandler)
	if err := events.HandleRepoStream(ctx, conn, sched, nil); err != nil {
		return fmt.Errorf("handle labeler stream: %w", err)
	}
	return nil
}

func (s *Subscriber) callbacks(subscriptionURL string) *events.RepoStreamCallbacks {
	return &events.RepoStreamCallbacks{
		LabelLabels: func(evt *comatproto.LabelSubscribeLabels_Labels) error {
			inputs, err := ConvertLabels(evt.Labels)
			if err != nil {
				return s.recordAndReturnError(context.Background(), subscriptionURL, fmt.Errorf("convert labeler labels seq=%d: %w", evt.Seq, err))
			}
			if err := s.repo.PersistEvent(context.Background(), subscriptionURL, evt.Seq, inputs); err != nil {
				return s.recordAndReturnError(context.Background(), subscriptionURL, fmt.Errorf("persist labeler labels seq=%d: %w", evt.Seq, err))
			}
			return nil
		},
		RepoInfo: func(evt *comatproto.SyncSubscribeRepos_Info) error {
			return s.handleInfo(context.Background(), subscriptionURL, evt.Name, evt.Message)
		},
		LabelInfo: func(evt *comatproto.LabelSubscribeLabels_Info) error {
			return s.handleInfo(context.Background(), subscriptionURL, evt.Name, evt.Message)
		},
		Error: func(evt *events.ErrorFrame) error {
			errText := evt.Error
			if evt.Message != "" {
				errText += ": " + evt.Message
			}
			return s.recordAndReturnError(context.Background(), subscriptionURL, fmt.Errorf("labeler stream error: %s", errText))
		},
	}
}

func (s *Subscriber) handleInfo(ctx context.Context, subscriptionURL, name string, message *string) error {
	messageText := ""
	if message != nil {
		messageText = *message
	}

	if name == "OutdatedCursor" {
		errText := "labeler stream info: OutdatedCursor"
		if messageText != "" {
			errText += ": " + messageText
		}
		return s.recordAndReturnError(ctx, subscriptionURL, fmt.Errorf("%s", errText))
	}

	slog.Info("Labeler stream info", "url", subscriptionURL, "name", name, "message", messageText)
	return nil
}

func (s *Subscriber) recordAndReturnError(ctx context.Context, subscriptionURL string, err error) error {
	if updateErr := s.repo.UpdateError(ctx, subscriptionURL, err.Error()); updateErr != nil {
		slog.Warn("Failed to record labeler subscription error", "url", subscriptionURL, "error", updateErr)
	}
	return err
}

// ConvertLabels converts Indigo label stream labels into repository inputs.
func ConvertLabels(labels []*comatproto.LabelDefs_Label) ([]repositories.ExternalLabelInput, error) {
	inputs := make([]repositories.ExternalLabelInput, 0, len(labels))
	for i, label := range labels {
		if label == nil {
			return nil, fmt.Errorf("label %d is nil", i)
		}

		rawJSON, err := json.Marshal(label)
		if err != nil {
			return nil, fmt.Errorf("marshal label %d raw JSON: %w", i, err)
		}

		neg := false
		if label.Neg != nil {
			neg = *label.Neg
		}

		var sig *string
		if len(label.Sig) > 0 {
			encoded := base64.StdEncoding.EncodeToString([]byte(label.Sig))
			sig = &encoded
		}

		inputs = append(inputs, repositories.ExternalLabelInput{
			LabelIndex: int64(i),
			Src:        label.Src,
			URI:        label.Uri,
			CID:        label.Cid,
			Val:        label.Val,
			Neg:        neg,
			Cts:        label.Cts,
			Exp:        label.Exp,
			Sig:        sig,
			Ver:        label.Ver,
			RawJSON:    string(rawJSON),
		})
	}
	return inputs, nil
}

func buildSubscribeURL(rawURL string, cursor int64) (string, error) {
	if cursor < 0 {
		return "", fmt.Errorf("cursor must be non-negative: %d", cursor)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("cursor", strconv.FormatInt(cursor, 10))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.ReconnectMin <= 0 {
		cfg.ReconnectMin = defaultReconnectInitial
	}
	if cfg.ReconnectMax <= 0 {
		cfg.ReconnectMax = defaultReconnectCeiling
	}
	if cfg.ReconnectMax < cfg.ReconnectMin {
		cfg.ReconnectMax = cfg.ReconnectMin
	}
	if cfg.ReadLimit <= 0 {
		cfg.ReadLimit = defaultReadLimit
	}

	urls := make([]string, 0, len(cfg.URLs))
	seen := make(map[string]struct{}, len(cfg.URLs))
	for _, rawURL := range cfg.URLs {
		trimmed := strings.TrimSpace(rawURL)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		urls = append(urls, trimmed)
	}
	cfg.URLs = urls
	return cfg
}
