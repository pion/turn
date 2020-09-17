# Usage
This command generates credentials used by the Long-Term Credential Mechanism

Defined in [RFC5389-10.2](https://tools.ietf.org/search/rfc5389#section-10.2). The idea is to use the
expiry time of the credential as the username, and let the password contain some cryptographic hash
of a (server-side) shared-secret and the expiry time.

```bash
export SECRET=somesecret

# Build binaries
(cd examples/lt-cred-generator && go build .)
(cd examples/turn-server/lt-cred && go build .)
(cd examples/turn-client/udp && go build .)

# Start server
./examples/turn-server/lt-cred/lt-cred -public-ip=127.0.0.1 -authSecret=$SECRET

# Start client using generated credentials
./examples/lt-cred-generator/lt-cred-generator -authSecret=$SECRET | xargs -I{} ./examples/turn-client/udp/udp -host=127.0.0.1 -ping -user={}
```
