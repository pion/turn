# Pion TURN

A TURN server written in Go that is designed to be scalable, extendable and embeddable out of the box.
Instead of complicated config files or dependencies you get a single static binary that can be
configured via environment variables (or any format of your choice).

### Extendable
You can import it and add any authentication you want.  Instead of forcing you to use arbitrary
schemas and painful integrations it fits right into your existing system.

### Embeddable
Most architectures are designed with a TURN server being a dedicated service, this means you have more points of failure and more
cognitive load designing your systems. With Pion TURN you can just launch it from your existing Go code base, allowing you to ship
quicker and having less painful deploys.

## Getting Started
See [simple-turn](https://github.com/pions/turn/blob/master/cmd/simple-turn.go)

simple-turn is a TURN server that allows any user, as long as they provide the credential 'password'. Copy this file
and start making changes to create your own deployment. simple-turn also works great for simple use cases, if you want to control
everything via environment variables in a trusted environment.

### Prerequisites
Pion TURN is 100% Go, there are no other dependencies beyond a working Golang environment.

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

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md)

## License
MIT License - see [LICENSE.md](LICENSE.md) for full text
