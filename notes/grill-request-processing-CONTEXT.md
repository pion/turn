# TURN

This context describes the language used for messages and relay state in a TURN client or server.

## Language

**Inbound TURN message**:
A complete message received by a TURN server from a client, represented as either a STUN-formatted message or ChannelData.
_Avoid_: Request, packet

**TURN request**:
A STUN-formatted request asking a TURN server to perform a TURN method. Indications and ChannelData are not TURN requests.
_Avoid_: Inbound TURN message when the distinction matters
