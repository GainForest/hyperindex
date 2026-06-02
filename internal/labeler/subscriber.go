// Package labeler subscribes to external ATProto labeler streams.
package labeler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"github.com/GainForest/hyperindex/internal/logsafe"
)

const (
	defaultReconnectInitial = time.Second
	defaultReconnectCeiling = 60 * time.Second
	defaultReadLimit        = 4 << 20 // 4 MiB

	fatalCursorFuture   = "FutureCursor"
	fatalCursorOutdated = "OutdatedCursor"
)

// FatalCursorError describes a non-retryable labeler cursor error. These
// errors mean the saved cursor cannot be used with the current labeler stream
// and require an operator reset instead of another reconnect attempt.
type FatalCursorError struct {
	Code    string
	Message string
}

func (e *FatalCursorError) Error() string {
	if e == nil {
		return "fatal cursor error"
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Config configures external labeler websocket subscriptions.
type Config struct {
	URLs         []string
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	ReadLimit    int64
}

type subscriptionRepository interface {
	EnsureState(context.Context, string) (*repositories.LabelSubscriptionState, error)
	UpdateConnected(context.Context, string) error
	UpdateError(context.Context, string, string) error
	PersistEvent(context.Context, string, int64, []repositories.ExternalLabelInput) error
	MarkFatalCursor(context.Context, string, string, string) error
}

// Subscriber manages one external labeler websocket subscription per URL.
type Subscriber struct {
	repo   subscriptionRepository
	cfg    Config
	dialer *websocket.Dialer
}

// NewSubscriber creates a labeler subscriber.
func NewSubscriber(repo *repositories.ExternalLabelsRepository, cfg Config) *Subscriber {
	return newSubscriber(repo, cfg)
}

func newSubscriber(repo subscriptionRepository, cfg Config) *Subscriber {
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

		var fatalErr *FatalCursorError
		if errors.As(err, &fatalErr) {
			slog.Error("Labeler subscription stopped after fatal cursor error", "url", logsafe.URL(subscriptionURL), "error_code", fatalErr.Code, "error", err)
			return
		}

		if err != nil {
			slog.Error("Labeler subscription disconnected", "url", logsafe.URL(subscriptionURL), "error", err, "backoff", backoff)
			if updateErr := s.repo.UpdateError(ctx, subscriptionURL, err.Error()); updateErr != nil {
				slog.Warn("Failed to record labeler subscription error", "url", logsafe.URL(subscriptionURL), "error", updateErr)
			}
		} else {
			slog.Warn("Labeler subscription closed unexpectedly", "url", logsafe.URL(subscriptionURL), "backoff", backoff)
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
	if state.IsFatal() {
		code := repositories.FatalCursorCode(state.LastError)
		if code == "" {
			code = "FatalCursor"
		}
		fatal := &FatalCursorError{Code: code, Message: "subscription is marked fatal. Reset subscription cursor and replay labels before labels can resume"}
		return fmt.Errorf("labeler subscription requires reset: %w", fatal)
	}

	streamURL, err := buildSubscribeURL(subscriptionURL, state.LastSeq)
	if err != nil {
		return fmt.Errorf("build labeler subscription URL: %w", err)
	}

	slog.Info("Connecting to labeler subscription", "url", logsafe.URL(subscriptionURL), "cursor", state.LastSeq)
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

	callbacks := s.callbacks(ctx, subscriptionURL)
	sched := sequential.NewScheduler("labeler:"+subscriptionURL, callbacks.EventHandler)
	if err := events.HandleRepoStream(ctx, conn, sched, nil); err != nil {
		return fmt.Errorf("handle labeler stream: %w", err)
	}
	return nil
}

func (s *Subscriber) callbacks(ctx context.Context, subscriptionURL string) *events.RepoStreamCallbacks {
	return &events.RepoStreamCallbacks{
		LabelLabels: func(evt *comatproto.LabelSubscribeLabels_Labels) error {
			inputs, err := ConvertLabels(evt.Labels)
			if err != nil {
				return s.recordAndReturnError(ctx, subscriptionURL, fmt.Errorf("convert labeler labels seq=%d: %w", evt.Seq, err))
			}
			if err := s.repo.PersistEvent(ctx, subscriptionURL, evt.Seq, inputs); err != nil {
				return s.recordAndReturnError(ctx, subscriptionURL, fmt.Errorf("persist labeler labels seq=%d: %w", evt.Seq, err))
			}
			return nil
		},
		RepoInfo: func(evt *comatproto.SyncSubscribeRepos_Info) error {
			return s.handleInfo(ctx, subscriptionURL, evt.Name, evt.Message)
		},
		LabelInfo: func(evt *comatproto.LabelSubscribeLabels_Info) error {
			return s.handleInfo(ctx, subscriptionURL, evt.Name, evt.Message)
		},
		Error: func(evt *events.ErrorFrame) error {
			if evt.Error == fatalCursorFuture {
				return s.recordFatalCursorAndReturnError(ctx, subscriptionURL, fatalCursorFuture, evt.Message)
			}

			errText := evt.Error
			if evt.Message != "" {
				errText += ": " + evt.Message
			}
			return s.recordAndReturnError(ctx, subscriptionURL, fmt.Errorf("labeler stream error: %s", errText))
		},
	}
}

func (s *Subscriber) handleInfo(ctx context.Context, subscriptionURL, name string, message *string) error {
	messageText := ""
	if message != nil {
		messageText = *message
	}

	if name == fatalCursorOutdated {
		return s.recordFatalCursorAndReturnError(ctx, subscriptionURL, fatalCursorOutdated, messageText)
	}

	slog.Info("Labeler stream info", "url", logsafe.URL(subscriptionURL), "name", name, "message", messageText)
	return nil
}

func (s *Subscriber) recordAndReturnError(ctx context.Context, subscriptionURL string, err error) error {
	if updateErr := s.repo.UpdateError(ctx, subscriptionURL, err.Error()); updateErr != nil {
		slog.Warn("Failed to record labeler subscription error", "url", logsafe.URL(subscriptionURL), "error", updateErr)
	}
	return err
}

func (s *Subscriber) recordFatalCursorAndReturnError(ctx context.Context, subscriptionURL, code, message string) error {
	fatalErr := &FatalCursorError{Code: code, Message: fatalCursorResetMessage(code, message)}
	if updateErr := s.repo.MarkFatalCursor(ctx, subscriptionURL, code, fatalErr.Message); updateErr != nil {
		slog.Warn("Failed to record fatal labeler subscription error", "url", logsafe.URL(subscriptionURL), "error", updateErr)
		return fmt.Errorf("record fatal labeler subscription error %s: %w", code, updateErr)
	}
	return fatalErr
}

func fatalCursorResetMessage(code, message string) string {
	base := "Reset subscription cursor and replay labels."
	message = strings.TrimSpace(message)
	if message == "" {
		if code == fatalCursorFuture {
			return "Cursor is in the future. " + base
		}
		return "Cursor is outside retained history. " + base
	}
	if !strings.HasSuffix(message, ".") && !strings.HasSuffix(message, "!") && !strings.HasSuffix(message, "?") {
		message += "."
	}
	return message + " " + base
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
