package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
)

type GPGSigner struct {
	entity    *openpgp.Entity
	publicKey []byte
}

func NewGPGSigner(armoredPrivateKey string) (*GPGSigner, error) {
	entityList, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armoredPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("reading GPG key: %w", err)
	}
	if len(entityList) == 0 {
		return nil, fmt.Errorf("no GPG keys found")
	}
	entity := entityList[0]

	var pubBuf bytes.Buffer
	w, err := armor.Encode(&pubBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, fmt.Errorf("creating armor encoder: %w", err)
	}
	if err := entity.Serialize(w); err != nil {
		return nil, fmt.Errorf("serializing public key: %w", err)
	}
	w.Close()

	return &GPGSigner{
		entity:    entity,
		publicKey: pubBuf.Bytes(),
	}, nil
}

func (g *GPGSigner) PublicKey() []byte {
	return g.publicKey
}

func (g *GPGSigner) ClearSign(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := clearsign.Encode(&buf, g.entity.PrivateKey, nil)
	if err != nil {
		return nil, fmt.Errorf("clearsign encode: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		return nil, fmt.Errorf("clearsign write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("clearsign close: %w", err)
	}
	return buf.Bytes(), nil
}

func (g *GPGSigner) DetachedSign(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&buf, g.entity, bytes.NewReader(content), nil); err != nil {
		return nil, fmt.Errorf("detached sign: %w", err)
	}
	return buf.Bytes(), nil
}
