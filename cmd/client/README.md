# client

Client is a simple TURN client. It sends a STUN binding request to the TURN server and prints the result.

## Usage

### Native
First install the client:
```sh
go get github.com/pions/turn
go install github.com/pions/turn/cmd/client
```
Next, run the executable. Note: This assumes `$GOPATH/bin` is part of your `$PATH`.
```sh
client
```
You can pass arguments as follows:
```sh
client -host 127.0.0.1 -port 3478
```

### Docker
First build the container:
```sh
docker build -t turn-client -f Dockerfile.production .
```
Next run the container:
```sh
docker run turn-client
```
You can pass arguments as follows:
```sh
docker run turn-client -host 127.0.0.1 -port 3478
```