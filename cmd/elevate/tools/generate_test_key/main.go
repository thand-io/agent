package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		outDir    string
		keyID     string
		overwrite bool
	)

	flag.StringVar(&outDir, "out-dir", ".", "output directory for generated key files")
	flag.StringVar(&keyID, "key-id", "manual-test-key", "key id used for file names")
	flag.BoolVar(&overwrite, "overwrite", false, "overwrite existing files")
	flag.Parse()

	if keyID == "" {
		exitf("key-id cannot be empty")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		exitf("create output directory: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		exitf("generate keypair: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		exitf("marshal public key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		exitf("marshal private key: %v", err)
	}

	publicPath := filepath.Join(outDir, keyID+".pem")
	privatePath := filepath.Join(outDir, keyID+".private.pem")

	if err := writePEMFile(publicPath, "PUBLIC KEY", pubDER, 0o644, overwrite); err != nil {
		exitf("write public key: %v", err)
	}
	if err := writePEMFile(privatePath, "PRIVATE KEY", privDER, 0o600, overwrite); err != nil {
		exitf("write private key: %v", err)
	}

	fmt.Printf("generated public key: %s\n", publicPath)
	fmt.Printf("generated private key: %s\n", privatePath)
}

func writePEMFile(path, blockType string, der []byte, mode os.FileMode, overwrite bool) error {
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		return err
	}
	return f.Close()
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
