package ltcred

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	loglib "log"
	"net"
	"strconv"
	"time"

	"github.com/pion/turn/v2"
)

func longTermCredentials(username string, sharedSecret string) string {
	mac := hmac.New(sha1.New, []byte(sharedSecret))
	mac.Write([]byte(username))
	password := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(password)
}

// NewAuthHandler returns a turn.AuthAuthHandler used with Long Term (or Time Windowed) Credentials.
func NewAuthHandler(sharedSecret string, log *loglib.Logger) turn.AuthHandler {
	return func(username, realm string, srcAddr net.Addr) (key []byte, ok bool) {
		log.Printf("Authentication username=%q realm=%q srcAddr=%v\n", username, realm, srcAddr)
		t, err := strconv.Atoi(username)
		if err != nil {
			log.Printf("Invalid time-windowed username %q", username)
			return nil, false
		}
		if int64(t) < time.Now().Unix() {
			log.Printf("Expired time-windowed username %q", username)
			return nil, false
		}
		password := longTermCredentials(username, sharedSecret)
		return turn.GenerateAuthKey(username, realm, password), true
	}
}
