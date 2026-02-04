package cookie

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/kayrus/gof5/pkg/config"

	"golang.org/x/crypto/scrypt"
	"gopkg.in/yaml.v2"
)

const cookiesName = "cookies.yaml"
const cookieEncPrefix = "ENCv1:"

func parseCookies(configPath string, key []byte) (map[string][]string, bool, error) {
	cookies := make(map[string][]string)

	cookiesPath := filepath.Join(configPath, cookiesName)

	v, err := ioutil.ReadFile(cookiesPath)
	if err != nil {
		// skip "no such file or directory" error on the first startup
		if e, ok := err.(*os.PathError); !ok || e.Unwrap() != syscall.ENOENT {
			log.Printf("Cannot read cookies file: %v", err)
		}
		return cookies, false, nil
	}

	encrypted := false
	if strings.HasPrefix(string(v), cookieEncPrefix) {
		encrypted = true
		if len(key) == 0 {
			return nil, true, fmt.Errorf("cookies file is encrypted; set GOF5_COOKIE_KEY or use --cookie-key-stdin")
		}
		raw, err := decryptCookies(v, key)
		if err != nil {
			return nil, true, fmt.Errorf("failed to decrypt cookies: %v", err)
		}
		v = raw
	}

	if err = yaml.Unmarshal(v, &cookies); err != nil {
		return nil, encrypted, fmt.Errorf("cannot parse cookies: %v", err)
	}

	return cookies, encrypted, nil
}

func ReadCookies(c *http.Client, u *url.URL, cfg *config.Config, sessionID string, key []byte) error {
	v, encrypted, err := parseCookies(cfg.Path, key)
	if err != nil {
		return err
	}
	if !encrypted && len(key) == 0 {
		log.Printf("Warning: cookies file is stored in plaintext; set GOF5_COOKIE_KEY to enable encryption")
	}
	if v, ok := v[u.Host]; ok {
		var cookies []*http.Cookie
		for _, c := range v {
			if v := strings.Split(c, "="); len(v) == 2 {
				cookies = append(cookies, &http.Cookie{Name: v[0], Value: v[1]})
			}
		}
		c.Jar.SetCookies(u, cookies)
	}

	if sessionID != "" {
		log.Printf("Overriding session ID from a CLI argument")
		// override session ID from CLI parameter
		cookies := []*http.Cookie{
			{Name: "MRHSession", Value: sessionID},
		}
		c.Jar.SetCookies(u, cookies)
	}
	return nil
}

func SaveCookies(c *http.Client, u *url.URL, cfg *config.Config, key []byte, allowPlaintext bool) error {
	raw, _, err := parseCookies(cfg.Path, key)
	if err != nil {
		return err
	}
	// empty current cookies list
	raw[u.Host] = nil
	// write down new cookies
	for _, c := range c.Jar.Cookies(u) {
		raw[u.Host] = append(raw[u.Host], c.String())
	}

	cookies, err := yaml.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("cannot marshal cookies: %v", err)
	}

	cookiesPath := filepath.Join(cfg.Path, cookiesName)
	if len(key) == 0 {
		if !allowPlaintext {
			return fmt.Errorf("refusing to write plaintext cookies; set GOF5_COOKIE_KEY or use --no-store-cookies")
		}
		if err = ioutil.WriteFile(cookiesPath, cookies, 0600); err != nil {
			return fmt.Errorf("failed to save cookies: %s", err)
		}
	} else {
		enc, err := encryptCookies(cookies, key)
		if err != nil {
			return err
		}
		if err = ioutil.WriteFile(cookiesPath, enc, 0600); err != nil {
			return fmt.Errorf("failed to save cookies: %s", err)
		}
	}

	if runtime.GOOS != "windows" {
		if err = os.Chown(cookiesPath, cfg.Uid, cfg.Gid); err != nil {
			return fmt.Errorf("failed to set an owner for cookies file: %s", err)
		}
	}

	return nil
}

func encryptCookies(plain, key []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %v", err)
	}
	derived, err := scrypt.Key(key, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %v", err)
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %v", err)
	}
	ct := gcm.Seal(nil, nonce, plain, nil)
	payload := append(salt, nonce...)
	payload = append(payload, ct...)
	enc := base64.StdEncoding.EncodeToString(payload)
	return []byte(cookieEncPrefix + enc), nil
}

func decryptCookies(enc, key []byte) ([]byte, error) {
	raw := strings.TrimPrefix(string(enc), cookieEncPrefix)
	payload, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cookies payload: %v", err)
	}
	if len(payload) < 16 {
		return nil, fmt.Errorf("cookies payload too short")
	}
	salt := payload[:16]
	derived, err := scrypt.Key(key, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %v", err)
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %v", err)
	}
	if len(payload) < 16+gcm.NonceSize() {
		return nil, fmt.Errorf("cookies payload too short")
	}
	nonce := payload[16 : 16+gcm.NonceSize()]
	ct := payload[16+gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt cookies: %v", err)
	}
	return plain, nil
}

func RemoveCookiesFile(cfg *config.Config) error {
	cookiesPath := filepath.Join(cfg.Path, cookiesName)
	if err := os.Remove(cookiesPath); err != nil {
		return fmt.Errorf("failed to remove cookies file: %s", err)
	}
	return nil
}
