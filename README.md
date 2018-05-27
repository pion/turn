# Pion TURN
[![Build Status](https://travis-ci.org/pions/turn.svg?branch=master)](https://travis-ci.org/pions/turn)
[![GoDoc](https://godoc.org/github.com/pions/turn?status.svg)](https://godoc.org/github.com/pions/turn)
[![Go Report Card](https://goreportcard.com/badge/github.com/pions/turn)](https://goreportcard.com/report/github.com/pions/turn)
[![Coverage Status](https://coveralls.io/repos/github/pions/turn/badge.svg)](https://coveralls.io/github/pions/turn)
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/d53ec6c70576476cb16c140c2964afde)](https://www.codacy.com/app/Sean-Der/turn?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=pions/turn&amp;utm_campaign=Badge_Grade)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.md)

<div align="center">
    <a href="#">
        <img src="./.github/gopher-pion.png" height="300px">
    </a>
</div>

A TURN server written in Go that is designed to be scalable, extendable and embeddable out of the box.
For simple use cases it only requires downloading 1 static binary, and setting 3 options.

See [DESIGN.md](DESIGN.md) for the the features it offers, and future goals.

## Getting Started
### Quick Start
If you want just a simple TURN server with a few static usernames `simple-turn` will perfectly suit your purposes. If you have
custom requirements such as a database proceed to extending.

`simple-turn` is a single static binary, and all config is driven by environment variables. On a fresh Linux AWS instance these are all the steps you would need.
```
$ wget -q https://github.com/pions/turn/releases/download/1.0.3/simple-turn-linux-amd64
$ chmod +x simple-turn-linux-amd64
$ export USERS='user=password foo=bar'
$ export REALM=my-server.com
$ export UDP_PORT=3478
$ ./simple-turn-linux-amd64
````

To explain what every step does
* Download simple-turn for Linux x64, see [release](https://github.com/pions/turn/releases) for other platforms
* Make it executable
* Configure auth, in the form of `USERNAME=PASSWORD USERNAME=PASSWORD` with no limits
* Set your realm, this is the public URL or name of your server
* Set the port you listen on, 3478 is the default port for TURN

That is it! Then to use your new TURN server your WebRTC config would look like
```
{ iceServers: [{
  urls: "turn:YOUR_SERVER"
  username: "user",
  credential: "password"
}]
```
---

If you are using Windows you would set these values in Powershell by doing. Also make sure your firewall is configured properly.
```
> $env:USERS = "user=password foo=bar"
> $env:REALM = "my-server.com"
> $env:UDP_PORT = 3478
```
### Extending
See [simple-turn](https://github.com/pions/turn/blob/master/cmd/simple-turn/main.go)

pion-turn can be configured by implementing [these callbacks](https://github.com/pions/turn/blob/master/turn.go#L11) and by passing [these arguments](https://github.com/pions/turn/blob/master/turn.go#L11)

All that `simple-turn` does is take environment variables, and then uses the same API.


### Developing
For developing a Dockerfile is available with features like hot-reloads, and is meant to be volume mounted.
Make sure you also have github.com/pions/pkg in your path, or you can exclude the second volume mount.

This is only meant for development, see [demo-conference](https://github.com/pions/demo-conference)
to see TURN usage as a user.
```
docker build -t turn .
docker run -v $(pwd):/usr/local/src/github.com/pions/turn -v $(pwd)/../pkg:/usr/local/src/github.com/pions/pkg turn
```

Currently only Linux is supported until Docker supports full (host <-> container) networking on Windows/OSX

## RFCs
### Implemented
* [RFC 5389: Session Traversal Utilities for NAT (STUN)](https://tools.ietf.org/html/rfc5389)
* [RFC 5766: Traversal Using Relays around NAT (TURN)](https://tools.ietf.org/html/rfc5766)

### Planned
* [RFC 6062: Traversal Using Relays around NAT (TURN) Extensions for TCP Allocations](https://tools.ietf.org/html/rfc6062)
* [RFC 6156: Traversal Using Relays around NAT (TURN) Extension for IPv6](https://tools.ietf.org/html/rfc6156)

## Questions/Support
Sign up for the [Golang Slack](https://invite.slack.golangbridge.org/) and join the #pion channel for discussions and support

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md)

### Contributors

* [Sean DuBois](https://github.com/Sean-Der) - *Original Author*  
* [winds2016](https://github.com/winds2016) - *Windows platform testing*  
* [Ingmar Wittkau](https://github.com/iwittkau) - *STUN client*

## License
MIT License - see [LICENSE.md](LICENSE.md) for full text
