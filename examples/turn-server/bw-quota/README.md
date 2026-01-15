# Bandwidth Quota TURN Server Example

This example demonstrates how to implement per-user bandwidth quotas in the TURN server. Each user (identified by username+realm) gets their own rate limiter that caps their total bandwidth usage across all relay connections.

``` mermaid
flowchart LR
    A[iperf-client] --> B[TURN client-proxy]
    B --> C[TURN server]
    C -->|rate-limiter| D[iperf server]
```

## Usage

### Start the Server

```bash
go run main.go -public-ip=<YOUR_PUBLIC_IP> -users=user1=pass1,user2=pass2 -bw-limit=12500
```

**Flags:**
- `-public-ip`: Server's public IP address (required)
- `-port`: TURN server port (default: 3478)
- `-users`: Comma-separated list of user=password pairs (required)
- `-realm`: Authentication realm (default: "pion.ly")
- `-bw-limit`: Bandwidth limit in bytes/sec per user (default: 12500 = 100 Kbps)
- `-test`: Enable test mode with built-in TURN client and UDP echo server
- `-test-port`: UDP port for test echo server (default: 15000)

### Test Mode

The `-test` flag starts a built-in TURN client that allocates a relay and exposes a UDP echo server for easy testing:

```bash
go run main.go -public-ip=127.0.0.1 -users=testuser=testpass -bw-limit=12500 -test
```

## Testing with iperf

You can verify the bandwidth quota is working using `iperf` UDP mode.

### Setup

1. Start the TURN server, `-test` means to also start a TURN client that serves as a proxy for the
   load generator (`iperf` in this case):
   ```bash
   go run main.go -public-ip=127.0.0.1 -users=user1=pass1 -bw-limit=12500 -test
   ```

2. Start iperf server (simulates a peer):
   ```bash
   iperf -s -u -p 5001
   ```

3. Run a TURN client that connects to the peer through the relay. This will run for 10 seconds,
   generating 1 Mbps load traffic. The summary log however shows only ~100 Kbps throughput instead
   of the requested 1 Mbps as the rate-limiter drops traffic exceeding the bandwidth quota.
   ```bash
   iperf -c <proxy-address> -u -p <proxy-port> -b 1M -t 10
   [ ID] Interval       Transfer     Bandwidth        Jitter   Lost/Total Datagrams
   [  1] 0.0000-10.0596 sec   135 KBytes   110 Kbits/sec   0.000 ms 804/898 (0%)
   ```

   

