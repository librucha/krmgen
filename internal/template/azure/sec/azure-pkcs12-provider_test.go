package azsec

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestGetPfxKey(t *testing.T) {
	type args struct {
		vaultName string
		keyArgs   []string
	}
	tests := []struct {
		name      string
		args      args
		resBody   string
		resStatus int
		want      any
		wantErr   bool
	}{
		{
			name: "valid pfx",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id"},
			},
			resBody: `{"id":"https://vault_name.vault.azure.net/key_id/","value":"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"}`,
			want:    "-----BEGIN PRIVATE KEY-----\nMIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDZj4M4gsDSP1AF\nAv3cYaZiQGn1Q5AHWkIV3KfmsKq3nNlgRJ9qNqBKYZUxufPFZ8ux9PBWgwsvSzT9\neXcmsjUE6njQ6Kq4sDpdV4Qgc2cJ9YsbMKhS4sqzN0Ja6l+rzNVM5lc6m9oq9LCa\n6K8LW2O/ZwqdOommOIDfcS5kLfEcLQOlK9qPimQF/lR/5+6hRo+K7pe+p8/nNuTr\nAoPkv1UgZbDk4P35rmBCtTxdpQcOsVCbKoSrJClqb2vzN6f31RWEpuzITyO/43ri\n+STgubfNLcH8h9VShWgsypkNMdLomc0uDzCLWvjTKrSjWMcCTDWYFgIp8EBQTnp2\nthQcU71XAgMBAAECggEAO3fpDHdhMZcwzk3lCmp+yniE/g+7vObFDajFFF/SKmJr\nYM8hLC1GX06RM4h6w8j9euVTFLK5SfIqx+Z91Uv9Bhz5bVFL6TPyoDUd3qjsz2IY\n5hPEzvNDKP2/244ZHKLe4yhLS6/yUK+V3qIfxuDyQQ1vb07i9VaYk3sijSuprmN2\nLbT5oGEUQubgz2SRRG6biLFIWV9KD5ah6EDjHa79PN+JhPiVDaASXp116Z+9bidt\nJoksxfwsiHeK3R22fUDQTPsoKg6kFqoRxZWl6oY0Iv2sWtpBxHfX0v/Fx0HsgZmK\nTVXn+MYPCU4WBd2Lfbeb81pyMHutfJh460/zaAF8gQKBgQDbG6qA82jNA/mu9U0C\nZRp0uRiLNc9WHKx8/I33KLhoJx8ngJKIdIOuMYx645CqJ6jyeJcnV8Nx4pMaVBjM\nkU6yjT5Vt+D8ubS+izM6qRTzbZOaB2y2xjMrUCTzYIeCuh9h99VbB6typFPtOd4P\nQGFOpPePvPf3LJkQ9ttApzaq4QKBgQD+MSUZoFKRjJdUFOriC8IDOMXxZt4zNk9H\nc2cY2+SzeIHO3W5pirIG1BWDYimGvx8SKtS16QOtqT39WvmmdGWNq1y9vtk+TIv1\n8ELSLaN4s9VFXBplXIoUsY18MFBJVDRW73TVyl2zEHD7qKZI0Qy/LgTJq5BsO6j8\nfp0GC+TnNwKBgCPPTruKjKtNJgaRMsfcbEl9YuSFo+BICWzX/f/SGOl002OqYMiK\nemcC1BnVjXQxzSvrx5B3iIrZY/9elTsB2KHX8cMirVPAqiimKXZB4hmy4/e9lOf+\nVqiSjad1NFCKSMzDK4yYIU44SzsvRPqrI/wtfARy9vffwxiBr+3OJmIhAoGADlPa\n0Xz16npQNU8Qhjk/cEsM7TRtJdnT0iUxFHeghnUua+iTRqOosTXXGJa53Hx9VdrQ\nLoi5yloVwmgUVkuNRdT430EYoahS40PtoEcuRaltRgGRA1GZ/tybKvrWK6vxX00T\n+tDzQxqUI7s31DbkTwpa/rsK4u7h8Yl5dFPLTTUCgYAFM84igWBqZc/f2+TVDJMG\nrTMm+S14IrvWCz/l0Yp7J7f4Bb606uFiVQDpp8QZm0tr6qQJgPmg6Pe9f3yHXpE9\nPFOpQK5QTpNcTdAp5nLvEjJvVkP3oWiixDoW+2KKXDIXftNea1fcLboFy+8Qq92e\n3Qca3hXTTWWv9KhLSOZ28g==\n-----END PRIVATE KEY-----\n",
			wantErr: false,
		},
	}

	// Setup client and mock Sender
	sender := &mockSender{}

	options := azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azsecrets.NewClient("https://fake.vault.io", &FakeCredential{}, &options)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			azureClients[tt.args.vaultName] = client
			cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 10)

			if tt.resStatus == 0 {
				tt.resStatus = http.StatusOK
			}

			headers := http.Header{}
			headers.Set("WWW-Authenticate", `Bearer authorization="https://login.windows.net/d5069782-a6df-436e-bac4-67b0c78175c8", resource="not_empty"`)

			sender.doFunc = func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.resStatus,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(tt.resBody)),
				}, nil
			}

			got, err := GetPfxKey(tt.args.vaultName, tt.args.keyArgs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPfxKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetPfxKey() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPfxCert(t *testing.T) {
	type args struct {
		vaultName string
		keyArgs   []string
	}
	tests := []struct {
		name      string
		args      args
		resBody   string
		resStatus int
		want      any
		wantErr   bool
	}{
		{
			name: "valid pfx",
			args: args{
				vaultName: "vault_name",
				keyArgs:   []string{"key_id"},
			},
			resBody: `{"id":"https://vault_name.vault.azure.net/key_id/","value":"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"}`,
			want:    "-----BEGIN CERTIFICATE-----\nMIIC1DCCAbygAwIBAgIBATANBgkqhkiG9w0BAQsFADASMRAwDgYDVQQKEwdBY21l\nIENvMB4XDTA5MTExMDIzMDAwMFoXDTEwMDUwOTIzMDAwMFowEjEQMA4GA1UEChMH\nQWNtZSBDbzCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBANmPgziCwNI/\nUAUC/dxhpmJAafVDkAdaQhXcp+awqrec2WBEn2o2oEphlTG588Vny7H08FaDCy9L\nNP15dyayNQTqeNDoqriwOl1XhCBzZwn1ixswqFLiyrM3QlrqX6vM1UzmVzqb2ir0\nsJrorwtbY79nCp06iaY4gN9xLmQt8RwtA6Ur2o+KZAX+VH/n7qFGj4rul76nz+c2\n5OsCg+S/VSBlsOTg/fmuYEK1PF2lBw6xUJsqhKskKWpva/M3p/fVFYSm7MhPI7/j\neuL5JOC5t80twfyH1VKFaCzKmQ0x0uiZzS4PMIta+NMqtKNYxwJMNZgWAinwQFBO\nena2FBxTvVcCAwEAAaM1MDMwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsG\nAQUFBwMBMAwGA1UdEwEB/wQCMAAwDQYJKoZIhvcNAQELBQADggEBAIOoBzl+bftm\nIWtLQPXiDQ+/d1fgy2BnQ9C9p5JYcsvrHBz5IHXMI+k04sLcwMJgPv005J0fQACX\nfiEuW1zytL6wMhHZa0eUzu/EvIzd/0+mg+T4mgUH5bl+zRDgyMZVZBAtKb2rk+pn\nax3F2j6xeBDnFYtSeY///GRLWEvZ7qAUELuEAj4YgbJbb7payg3ZhTWq3SutJbbm\nYKWIxpn5hSqEDslAzPRJo2iVp9aboZXLB5r7v8yrFlvwoCx7v1LC5aa4ZOP86pSC\njQWpWaiF8T9yFnnlqOZD+0ZJ2EjPOSddfC7tNOXwAz0o3TdDgSGpnTSRYZhHL2D+\nNeh/bCEQw/8=\n-----END CERTIFICATE-----\n\n",
			wantErr: false,
		},
	}

	// Setup client and mock Sender
	sender := &mockSender{}

	options := azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: sender,
		},
		DisableChallengeResourceVerification: true,
	}
	client, _ := azsecrets.NewClient("https://fake.vault.io", &FakeCredential{}, &options)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			azureClients[tt.args.vaultName] = client
			cachedSecrets = make(map[azsecrets.ID]*azsecrets.Secret, 10)

			if tt.resStatus == 0 {
				tt.resStatus = http.StatusOK
			}

			headers := http.Header{}
			headers.Set("WWW-Authenticate", `Bearer authorization="https://login.windows.net/d5069782-a6df-436e-bac4-67b0c78175c8", resource="not_empty"`)

			sender.doFunc = func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.resStatus,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(tt.resBody)),
				}, nil
			}

			got, err := GetPfxCert(tt.args.vaultName, tt.args.keyArgs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPfxCert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetPfxCert() got = %v, want %v", got, tt.want)
			}
		})
	}
}
