package jwks

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

func LoadEd25519PrivateKeyPEM(pemBytes []byte) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, nil, errors.New("invalid PEM: no block found")
	}
	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse pkcs8: %w", err)
		}
		priv, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, nil, errors.New("pkcs8 key is not Ed25519")
		}
		return priv, priv.Public().(ed25519.PublicKey), nil
	case "ED25519 PRIVATE KEY":
		switch len(block.Bytes) {
		case ed25519.PrivateKeySize:
			priv := ed25519.PrivateKey(block.Bytes)
			return priv, priv.Public().(ed25519.PublicKey), nil
		case ed25519.SeedSize:
			priv := ed25519.NewKeyFromSeed(block.Bytes)
			return priv, priv.Public().(ed25519.PublicKey), nil
		default:
			return nil, nil, errors.New("invalid ED25519 PRIVATE KEY length")
		}
	default:
		return nil, nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}

func LoadEd25519PrivateKeyFile(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return LoadEd25519PrivateKeyPEM(raw)
}

func LoadEd25519PublicKeyPEM(pemBytes []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM: no block found")
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkix: %w", err)
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("pkix key is not Ed25519")
	}
	return pub, nil
}

func MarshalEd25519PrivateKeyPEM(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func MarshalEd25519PublicKeyPEM(pub ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
