# TURN

This context describes the language used for relay state owned by a TURN server.

## Language

**Allocation**:
The relay state associated with one client-server five-tuple. It owns a relay transport address and the permissions, channel bindings, and peer data connections created beneath it.
_Avoid_: Session

**Five-tuple**:
The client transport address, server transport address, and transport protocol that together identify an allocation.
_Avoid_: Connection ID

**Relay transport address**:
The server transport address allocated for communication between a client and its peers.
_Avoid_: Relay address when the transport distinction matters

**Permission**:
An allocation-scoped authorization to communicate with a peer IP address, independent of the peer port.
_Avoid_: Peer connection

**Channel binding**:
An allocation-scoped association between one channel number and one peer transport address. A channel binding also creates or refreshes the required permission.
_Avoid_: Permission

**Peer data connection**:
A TCP connection between a TURN server and a peer, associated with a TCP allocation and identified to the client by a connection ID.
_Avoid_: Control connection

**Reservation token**:
A short-lived value that identifies a reserved relay transport address for a subsequent allocation.
_Avoid_: Connection ID
