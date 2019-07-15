# Examples

## turn-server
A simple TURN server.
It demonstrates how to write a TURN server and run it.

It takes a few environment variables:
* USERS   : &lt;username&gt;=&lt;password&gt;[,&lt;username&gt;=&lt;password&gt;,...] pairs
* REALM   : Realm name (defaults to "pion.ly")
* UDP_PORT: Listening port (defaults to 3478)


```sh
$ go build
$ PIONS_LOG_INFO=all USERS=user=pass REALM=pion.ly ./turn-server
turn INFO: 2019/07/14 14:40:17 Listening on udp:10.0.0.135:3478
```

## turn-client
A simple TURN client.
It demonstrates how to write a TURN client and run it.

The following command simply allocates a relay socket (relayConn), then exits.

```sh
$ go build
./turn-client -host <turn-server-name> -user=user=pass
```

By adding `-ping`, it will perform a ping test. (it internally creates a 'pinger' and send
a UDP packet every second, 10 times then exits.

```sh
$ go build
./turn-client -host <turn-server-name> -user=user=pass -ping
```

Following diagram shows what turn-client does:
```
          +----------------+
          |   TURN Server  |
          |                |
          +---o--------o---+
  TURN port  /^      / ^\
       3478_/ |     /  | \_relayConn (*1)
              |    /   |
              |  _/    |
  mappedAddr_ | /      | ___external IP:port
       (*2)  \|v       |/    for pingerConn
          +---o--------o---+     (*3)
          |   |   NAT  |   |
          +----------------+
              |        |
  TURN    ___ |        | __pingerConn
  listen     \|        |/     (sends `ping` to relayConn)
  port    +---o--------o---+
  (conn)  |   turn-client  |
          +----------------+
```

> (*1) The relayConn actually lives in the local turn-client, but it acts as if it is
> listening on the TURN server. In fact, relayConn.LocalAddr() returns a transport address
> on which the TURN server is listening.

> (*2) For relayConn to send/receive packet to/from (*3), you will need to give relayConn permission
> to send/receive packet to/from the IP address. In the example code, this is done by sending a
> packet, "Hello" (content does not matter), to the mappedAddr. (assuming the IP address of
> mappedAddr and the external IP:port (*3) are the same) This process is known as
> "UDP hole punching" and TURN server exhibits "Address-restricted" behavior. Once it is done,
> packets coming from (*3) will be received by relayConn.
