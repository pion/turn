package turn

import (
	"testing"
)

func TestLtCredMech(t *testing.T) {
	username := "1599491771"
	sharedSecret := "foobar"

	expectedPassword := "Tpz/nKkyvX/vMSLKvL4sbtBt8Vs="
	actualPassword, _ := longTermCredentials(username, sharedSecret)
	if expectedPassword != actualPassword {
		t.Errorf("Expected %q, got %q", expectedPassword, actualPassword)
	}
}

// export SECRET=foobar
// secret=$SECRET && \
// time=$(date +%s) && \
// expiry=8400 && \
// username=$(( $time + $expiry )) &&\
// echo username:$username && \
// echo password : $(echo -n $username | openssl dgst -binary -sha1 -hmac $secret | openssl base64)
// username : 1599491771
// password : M+WLqSVjDc7kfj2U8ZUmk+hTQl8=
