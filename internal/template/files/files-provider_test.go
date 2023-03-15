package files

import "testing"

func TestReadFile(t *testing.T) {
	type args struct {
		args []string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "local file",
			args:    args{args: []string{"test-file.txt"}},
			want:    "files-provider_test content",
			wantErr: false,
		},
		{
			name:    "parent file",
			args:    args{args: []string{"../template.go"}},
			want:    "",
			wantErr: true,
		},
		{
			name:    "abs file path",
			args:    args{args: []string{"/etc/pwd"}},
			want:    "",
			wantErr: true,
		},
		{
			name:    "local file with fallback",
			args:    args{args: []string{"test-file.txt", "fallback"}},
			want:    "files-provider_test content",
			wantErr: false,
		},
		{
			name:    "parent file with fallback",
			args:    args{args: []string{"../template.go", "fallback"}},
			want:    "",
			wantErr: true,
		},
		{
			name:    "abs file path with fallback",
			args:    args{args: []string{"/etc/pwd", "fallback"}},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFile(tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReadFile() got = %v, want %v", got, tt.want)
			}
		})
	}
}
