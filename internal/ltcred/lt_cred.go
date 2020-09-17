package ltcred

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/pion/turn/v2"
)

func longTermCredentials(username string, sharedSecret string) (string, error) {
	mac := hmac.New(sha1.New, []byte(sharedSecret))
	_, err := mac.Write([]byte(username))
	if err != nil {
		return "", err // Not sure if this will ever happen
	}
	password := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(password), nil
}

// NewAuthHandler returns a turn.AuthAuthHandler used with Long Term (or Time Windowed) Credentials.
func NewAuthHandler(sharedSecret string, f Feedback) turn.AuthHandler {
	if f == nil {
		f = noFeedback(0)
	}
	return func(username, realm string, srcAddr net.Addr) (key []byte, ok bool) {
		f.Print(fmt.Sprintf("Authentication username=%q realm=%q srcAddr=%v\n", username, realm, srcAddr))
		t, err := strconv.Atoi(username)
		if err != nil {
			f.Print(fmt.Sprintf("Invalid time-windowed username %q", username))
			return nil, false
		}
		if int64(t) < time.Now().Unix() {
			f.Print(fmt.Sprintf("Expired time-windowed username %q", username))
			return nil, false
		}
		password, err := longTermCredentials(username, sharedSecret)
		if err != nil {
			f.Print(err)
			return nil, false
		}
		return turn.GenerateAuthKey(username, realm, password), true
	}
}
