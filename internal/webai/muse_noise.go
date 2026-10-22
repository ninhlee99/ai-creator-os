package webai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

type museNoiseXX struct {
	curve   ecdh.Curve
	eph     *ecdh.PrivateKey
	static  *ecdh.PrivateKey
	re      []byte
	rs      []byte
	h       []byte
	ck      []byte
	key     []byte
	nonce   uint64
	sendKey []byte
	recvKey []byte
}

func newNoiseXX() (*museNoiseXX, error) {
	curve := ecdh.X25519()
	ephemeral, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	static, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	protocol := []byte("Noise_XX_25519_AESGCM_SHA256")
	h := make([]byte, sha256.Size)
	if len(protocol) <= len(h) {
		copy(h, protocol)
	} else {
		sum := sha256.Sum256(protocol)
		h = sum[:]
	}
	return &museNoiseXX{curve: curve, eph: ephemeral, static: static, h: museHash(h), ck: append([]byte(nil), h...)}, nil
}

func (n *museNoiseXX) writeMessage1() ([]byte, error) {
	message := n.eph.PublicKey().Bytes()
	n.mixHash(message)
	return message, nil
}

func (n *museNoiseXX) readMessage2(message []byte) ([]byte, error) {
	if len(message) < 32+48+16 {
		return nil, errors.New("short handshake message")
	}
	n.re = append([]byte(nil), message[:32]...)
	n.mixHash(n.re)
	peer, err := n.curve.NewPublicKey(n.re)
	if err != nil {
		return nil, err
	}
	shared, err := n.eph.ECDH(peer)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	staticPlain, err := n.decryptAndHash(message[32:80])
	if err != nil || len(staticPlain) != 32 {
		return nil, errors.New("invalid peer static key")
	}
	n.rs = append([]byte(nil), staticPlain...)
	peerStatic, err := n.curve.NewPublicKey(n.rs)
	if err != nil {
		return nil, err
	}
	shared, err = n.eph.ECDH(peerStatic)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	if payload, err := n.decryptAndHash(message[80:]); err != nil || len(payload) != 0 {
		return nil, errors.New("invalid handshake payload")
	}

	msg3, err := n.encryptAndHash(n.static.PublicKey().Bytes())
	if err != nil {
		return nil, err
	}
	peerEphemeral, err := n.curve.NewPublicKey(n.re)
	if err != nil {
		return nil, err
	}
	shared, err = n.static.ECDH(peerEphemeral)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	emptyPayload, err := n.encryptAndHash(nil)
	if err != nil {
		return nil, err
	}
	msg3 = append(msg3, emptyPayload...)
	n.sendKey, n.recvKey = museNoiseHKDF(n.ck, nil)
	return msg3, nil
}

func (n *museNoiseXX) split() ([]byte, []byte) {
	return n.sendKey, n.recvKey
}

func (n *museNoiseXX) mixHash(data []byte) {
	n.h = museHash(append(append([]byte(nil), n.h...), data...))
}

func (n *museNoiseXX) mixKey(input []byte) {
	n.ck, n.key = museNoiseHKDF(n.ck, input)
	n.nonce = 0
}

func (n *museNoiseXX) encryptAndHash(plain []byte) ([]byte, error) {
	out, err := museNoiseEncryptWithAD(n.key, n.nonce, plain, n.h)
	if err != nil {
		return nil, err
	}
	n.nonce++
	n.mixHash(out)
	return out, nil
}

func (n *museNoiseXX) decryptAndHash(ciphertext []byte) ([]byte, error) {
	out, err := museNoiseDecryptWithAD(n.key, n.nonce, ciphertext, n.h)
	if err != nil {
		return nil, err
	}
	n.nonce++
	n.mixHash(ciphertext)
	return out, nil
}

func museNoiseHKDF(chainingKey, input []byte) ([]byte, []byte) {
	mac := hmac.New(sha256.New, chainingKey)
	_, _ = mac.Write(input)
	prk := mac.Sum(nil)
	mac = hmac.New(sha256.New, prk)
	_, _ = mac.Write([]byte{1})
	out1 := mac.Sum(nil)
	mac = hmac.New(sha256.New, prk)
	_, _ = mac.Write(out1)
	_, _ = mac.Write([]byte{2})
	return out1, mac.Sum(nil)
}

func museNoiseEncrypt(key []byte, nonce uint64, plain []byte) ([]byte, error) {
	return museNoiseEncryptWithAD(key, nonce, plain, nil)
}

func museNoiseDecrypt(key []byte, nonce uint64, encrypted []byte) ([]byte, error) {
	return museNoiseDecryptWithAD(key, nonce, encrypted, nil)
}

func museNoiseEncryptWithAD(key []byte, nonce uint64, plain, aad []byte) ([]byte, error) {
	aead, err := museAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, museNonce(nonce), plain, aad), nil
}

func museNoiseDecryptWithAD(key []byte, nonce uint64, encrypted, aad []byte) ([]byte, error) {
	aead, err := museAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, museNonce(nonce), encrypted, aad)
}

func museAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func museNonce(n uint64) []byte {
	var nonce [12]byte
	binary.LittleEndian.PutUint64(nonce[4:], n)
	return nonce[:]
}

func museHash(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
