package tap

import "testing"

func TestShouldPurgeIdentity(t *testing.T) {
	tests := []struct {
		name string
		ev   *IdentityEvent
		want bool
	}{
		{
			name: "active status does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "active"},
			want: false,
		},
		{
			name: "active status with false is_active does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: false, Status: "active"},
			want: false,
		},
		{
			name: "active status with missing is_active does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", Status: "active"},
			want: false,
		},
		{
			name: "empty status with false is_active does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: false},
			want: false,
		},
		{
			name: "deleted purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "deleted"},
			want: true,
		},
		{
			name: "deactivated purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "deactivated"},
			want: true,
		},
		{
			name: "suspended purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "suspended"},
			want: true,
		},
		{
			name: "takendown purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "takendown"},
			want: true,
		},
		{
			name: "mixed case status purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "Suspended"},
			want: true,
		},
		{
			name: "whitespace status purges",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "  takendown  "},
			want: true,
		},
		{
			name: "unknown status with true is_active does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: true, Status: "mystery"},
			want: false,
		},
		{
			name: "unknown status with false is_active does not purge",
			ev:   &IdentityEvent{DID: "did:plc:1", IsActive: false, Status: "mystery"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldPurgeIdentity(tt.ev)
			if got != tt.want {
				t.Fatalf("shouldPurgeIdentity() = %v, want %v", got, tt.want)
			}
		})
	}
}
