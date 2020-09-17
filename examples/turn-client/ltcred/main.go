package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pion/turn/v2/internal/ltcred"
)

// Outputs username & password according to the
// Long-Term Credential Mechanism (RFC5389-10.2: https://tools.ietf.org/search/rfc5389#section-10.2)
//
// Usage:
// $ ltcred | xargs -n 2 -I {} turn-client-tcp -host example.org -user={}={} -ping=true
//
func main() {
	authSecret := flag.String("authSecret", "", "Shared secret for the Long Term Credential Mechanism")
	flag.Parse()

	u, p, _ := ltcred.GenerateCredentials(*authSecret, time.Minute)
	os.Stderr.WriteString("username\tpassword\n")      // ignored by xargs
	os.Stdout.WriteString(fmt.Sprintf("%s\t%s", u, p)) // for use with xargs
	os.Stderr.WriteString("\n")                        // ignored by xargs
}
