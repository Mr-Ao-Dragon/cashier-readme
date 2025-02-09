package client

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nsheridan/cashier/lib"
	"github.com/nsheridan/cashier/testdata"
	"github.com/stretchr/testify/suite"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type ClientSuite struct {
	suite.Suite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

func (s *ClientSuite) TestSavePublicFiles_MissingPrefix() {
	s.Nil(SavePublicFiles("", nil, nil))
}

func (s *ClientSuite) TestSavePublicFiles_WriteKey() {
	c, _, _, _, _ := ssh.ParseAuthorizedKey(testdata.Cert)
	cert := c.(*ssh.Certificate)
	s.NotNil(cert)
	err := SavePublicFiles("test", cert, cert.Key)
	s.ErrorIs(err, os.ErrNotExist)
}

func (s *ClientSuite) TestSavePrivateFiles_MissingPrefix() {
	s.Nil(SavePrivateFiles("", nil, nil))
}

func (s *ClientSuite) TestSavePrivateFiles_InvalidKey() {
	s.Error(SavePrivateFiles("test", &ssh.Certificate{KeyId: "key"}, nil))
}

func TestLoadCert(t *testing.T) {
	priv, _ := ssh.ParseRawPrivateKey(testdata.Priv)
	key := priv.(*rsa.PrivateKey)
	pub, _ := ssh.NewPublicKey(&key.PublicKey)
	c := &ssh.Certificate{
		KeyId:       "test_key_12345",
		Key:         pub,
		CertType:    ssh.UserCert,
		ValidBefore: ssh.CertTimeInfinity,
		ValidAfter:  0,
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Error(err)
	}
	c.SignCert(rand.Reader, signer)
	a := agent.NewKeyring()
	if err := InstallCert(a, c, key); err != nil {
		t.Error(err)
	}
	listedKeys, err := a.List()
	if err != nil {
		t.Errorf("Error reading from agent: %v", err)
	}
	if len(listedKeys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(listedKeys))
	}
	if !bytes.Equal(listedKeys[0].Marshal(), c.Marshal()) {
		t.Error("Certs not equal")
	}
	for _, k := range listedKeys {
		exp := time.Unix(int64(c.ValidBefore), 0).String()
		want := fmt.Sprintf("%s [Expires %s]", c.KeyId, exp)
		if k.Comment != want {
			t.Errorf("key comment:\nwanted:%s\ngot: %s", want, k.Comment)
		}
	}
}

func TestSignGood(t *testing.T) {
	res := &lib.SignResponse{
		Status:   "ok",
		Response: string(testdata.Cert),
	}
	j, _ := json.Marshal(res)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, string(j))
	}))
	defer ts.Close()
	_, err := send(&lib.SignRequest{}, "token", ts.URL, true)
	if err != nil {
		t.Error(err)
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey(testdata.Pub)
	if err != nil {
		t.Error(err)
	}
	c := &Config{
		CA:       ts.URL,
		Validity: "24h",
	}
	cert, err := Sign(k, "token", c)
	if cert == nil && err != nil {
		t.Error(err)
	}
}

func TestSignBad(t *testing.T) {
	res := &lib.SignResponse{
		Status:   "error",
		Response: `{"response": "error"}`,
	}
	j, _ := json.Marshal(res)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, string(j))
	}))
	defer ts.Close()
	_, err := send(&lib.SignRequest{}, "token", ts.URL, true)
	if err != nil {
		t.Error(err)
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey(testdata.Pub)
	if err != nil {
		t.Error(err)
	}
	c := &Config{
		CA:       ts.URL,
		Validity: "24h",
	}
	cert, err := Sign(k, "token", c)
	if cert != nil && err == nil {
		t.Error(err)
	}
}
