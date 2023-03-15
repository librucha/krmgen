package types

import (
	"testing"
)

func TestConfig_HasHelm(t *testing.T) {
	type fields struct {
		Helm *Helm
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "Has helm",
			fields: fields{Helm: &Helm{Charts: &[]HelmChart{{Name: "helm"}}}},
			want:   true,
		},
		{
			name:   "Hasn't helm",
			fields: fields{Helm: nil},
			want:   false,
		},
		{
			name:   "Has nil helm charts",
			fields: fields{Helm: &Helm{nil}},
			want:   false,
		},
		{
			name:   "Has empty helm charts",
			fields: fields{Helm: &Helm{&[]HelmChart{}}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Helm: tt.fields.Helm,
			}
			if got := config.HasHelm(); got != tt.want {
				t.Errorf("HasHelm() = %v, want %v", got, tt.want)
			}
		})
	}
}
