# Pion TURN ![Build Status](https://travis-ci.org/pions/turn.svg?branch=master)

A TURN server written in Go that is designed to be scalable, extendable and embeddable out of the box.
Instead of complicated config files or dependencies you get a single static binary that can be
configured via environment variables (or any format of your choice).

## Getting Started
### Quick Start
If you want just a simple TURN server with a few static usernames `simple-turn` will perfectly suit your purposes. If you have
custom requirements such as a database proceed to extending.

`simple-turn` is a single static binary, and all config is driven by enviroment variables. On a fresh AWS instance these are all the steps you would need.
```
$ wget -q https://github.com/pions/turn/releases/download/1.0.0/simple-turn-linux-amd64
$ chmod +x simple-turn-linux-amd64
$ export USERS='user=password foo=bar'
$ export REALM=my-server.com
$ export UDP_PORT=3478
$ ./simple-turn-linux-amd64
````

To explain what every step does
* Download simple-turn
* Make it executable
* Set your users, this is in the form of 'USERNAME=PASSWORD USERNAME=PASSWORD' you can have as many as you want
* Set your realm, this should be the public name of your server
* Set the port you listen on, 3478 is the default

That is it! Then to use your new TURN server your WebRTC config would look like
```
{ iceServers: [{
  urls: "turn:YOUR_SERVER"
  username: "user",
  credential: "password"
}]
```
### Extending
See [simple-turn](https://github.com/pions/turn/blob/master/cmd/simple-turn.go)

pion-turn can be configured by implementing [these callbacks](https://github.com/pions/turn/blob/master/turn.go#L11) and by passing [these arguments](https://github.com/pions/turn/blob/master/turn.go#L11)

All that `simple-turn` does is take enviroment variables, and then uses the same API.


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

## Questions/Support
Sign up for the [Golang Slack](https://invite.slack.golangbridge.org/) and join the #pion channel for discussions and support

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md)

## License
MIT License - see [LICENSE.md](LICENSE.md) for full text
