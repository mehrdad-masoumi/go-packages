package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/mehrdad-masoumi/go-packages/auth"
)

func main() {
	out := flag.String("out", "jwt_ed25519_private.pem", "private key PEM path")
	pubOut := flag.String("pub", "jwt_ed25519_public.pem", "public key PEM path")
	flag.Parse()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(err)
	}
	privPEM, err := auth.MarshalEd25519PrivateKeyPEM(priv)
	if err != nil {
		fatal(err)
	}
	pubPEM, err := auth.MarshalEd25519PublicKeyPEM(priv.Public().(ed25519.PublicKey))
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, privPEM, 0o600); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*pubOut, pubPEM, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s\nwrote %s\n", *out, *pubOut)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "genkey: %v\n", err)
	os.Exit(1)
}
