package azsec

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/pkcs12"
	"strings"
)

const PfxKeyFunc = "azPfxKey"
const PfxCrtFunc = "azPfxCrt"

func GetPfxKey(vaultName string, keyArgs ...string) (any, error) {
	secret, err := GetSecret(vaultName, keyArgs...)
	if err != nil {
		return nil, err
	}
	return extractKey(fmt.Sprint(secret))
}

func GetPfxCert(vaultName string, keyArgs ...string) (any, error) {
	secret, err := GetSecret(vaultName, keyArgs...)
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

	for _, block := range blocks {
		if strings.Contains(block.Type, "KEY") {
			block.Headers = nil
			pkcs8, err := convertToPkcs8(block.Bytes)
			if err != nil {
				return nil, err
			}
			block.Bytes = pkcs8
			return string(pem.EncodeToMemory(block)), nil
		}
	}
	return nil, fmt.Errorf("none KEY block found")
}

func convertToPkcs8(pkcs1 []byte) ([]byte, error) {
	key, err := x509.ParsePKCS1PrivateKey(pkcs1)
	if err != nil {
		return nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pkcs8, nil
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
