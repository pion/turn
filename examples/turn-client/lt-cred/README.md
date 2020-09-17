# Usage

```bash
export SECRET=somesecret

# Build binaries
(cd examples/turn-client/lt-cred && go build .)
(cd examples/turn-server/lt-cred && go build .)
(cd examples/turn-client/tcp && go build .)

# Start server
./examples/turn-server/lt-cred/lt-cred -public-ip=127.0.0.1 -authSecret=$SECRET &

# Start client using generated credentials
./examples/turn-client/lt-cred/lt-cred -authSecret=$SECRET | xargs -I {} ./examples/turn-client/tcp/tcp -user="{}={}" -host=shard-1.eu.gcp.borrel.app -port=443 -ping -realm=meet.jitsi
```
