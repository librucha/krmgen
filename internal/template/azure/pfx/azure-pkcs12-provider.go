package azpfx

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	azsec "github.com/librucha/krmgen/internal/template/azure/sec"
	"golang.org/x/crypto/pkcs12"
	"strings"
)

const PfxKeyFunc = "azPfxKey"
const PfxCrtFunc = "azPfxCrt"

func GetPfxKey(vaultName string, keyArgs ...string) (any, error) {
	secret, err := azsec.GetSecret(vaultName, keyArgs...)
	if err != nil {
		return nil, err
	}
	return extractKey(fmt.Sprint(secret))
}

func GetPfxCert(vaultName string, keyArgs ...string) (any, error) {
	secret, err := azsec.GetSecret(vaultName, keyArgs...)
	if err != nil {
		return nil, err
	}
	return extractCert(fmt.Sprint(secret))
}

func extractKey(b64Content string) (any, error) {
	pfxData, err := base64.StdEncoding.DecodeString(b64Content)
	if err != nil {
		return nil, err
	}
	blocks, err := pkcs12.ToPEM(pfxData, "")
	if err != nil {
		return nil, err
	}

	for i := range blocks {
		block := blocks[i]
		if strings.Contains(block.Type, "KEY") {
			block.Headers = nil
			return string(pem.EncodeToMemory(block)), nil
		}
	}
	return nil, fmt.Errorf("none KEY block found")
}

func extractCert(b64Content string) (any, error) {
	pfxData, err := base64.StdEncoding.DecodeString(b64Content)
	if err != nil {
		return nil, err
	}
	blocks, err := pkcs12.ToPEM(pfxData, "")
	if err != nil {
		return nil, err
	}

	var certs strings.Builder

	for i := range blocks {
		block := blocks[i]
		if !strings.Contains(block.Type, "KEY") {
			block.Headers = nil
			certs.Write(pem.EncodeToMemory(block))
			certs.WriteRune('\n')
		}
	}

	if strings.TrimSpace(certs.String()) == "" {
		return nil, fmt.Errorf("none KEY block found")
	}
	return certs.String(), nil
}
