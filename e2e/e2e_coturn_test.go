// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build coturn && !js
// +build coturn,!js

package e2e

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	coturnServerPort   = 3478
	coturnConfigFile   = "/tmp/turnserver.conf"
	coturnStartupDelay = 1 * time.Second
)

// serverCoturn starts a coturn TURN server.
//
//nolint:varnamelen
func serverCoturn(m *testmgr) {
	go func() {
		m.serverMutex.Lock()
		defer m.serverMutex.Unlock()

		host, portStr, err := net.SplitHostPort(m.serverAddr)
		if err != nil {
			m.errChan <- fmt.Errorf("failed to parse serverAddr: %w", err)

			return
		}

		config := fmt.Sprintf(`
listening-port=%s
listening-ip=%s
relay-ip=%s
external-ip=%s
user=%s:%s
realm=%s
lt-cred-mech
fingerprint
no-tls
no-dtls
log-file=stdout
`, portStr, host, host, host, m.username, m.password, m.realm)

		configFile := fmt.Sprintf("/tmp/turnserver-%s.conf", m.serverAddr)
		err = os.WriteFile(configFile, []byte(config), 0o600)
		if err != nil {
			m.errChan <- err

			return
		}
		defer func() {
			_ = os.Remove(configFile)
		}()

		// Start coturn server
		//nolint:noctx,gosec // Not using CommandContext to avoid goroutine leaks, config file is test-created
		cmd := exec.Command("turnserver", "-c", configFile)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard

		if err := cmd.Start(); err != nil {
			m.errChan <- fmt.Errorf("failed to start coturn: %w (is coturn installed?)", err)

			return
		}

		// Ensure server has time to start
		time.Sleep(coturnStartupDelay)

		m.serverReady <- struct{}{}

		// Wait for context cancellation
		<-m.ctx.Done()

		// Kill and wait for process to fully exit
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Wait for process to clean up completely
		_ = cmd.Wait()

		// Small delay to let goroutines fully clean up
		time.Sleep(100 * time.Millisecond)

		m.serverDone <- nil
		close(m.serverDone)
	}()
}

// clientCoturn uses turnutils_uclient to test against a Pion TURN server.
//
//nolint:varnamelen
func clientCoturn(m *testmgr) {
	select {
	case <-m.serverReady:
		// OK
	case <-time.After(time.Second * 5):
		m.errChan <- errServerTimeout

		return
	}

	m.clientMutex.Lock()
	defer m.clientMutex.Unlock()

	// Use turnutils_uclient to connect to the Pion server
	// -v: verbose
	// -u: username
	// -w: password
	// -r: realm
	// -e: peer address (echo server)
	// -n: number of messages
	// -m: message size
	// -W: time to run (seconds)
	args := []string{
		// "-v",
		"-u", m.username,
		"-w", m.password,
		"-r", m.realm,
		"-n", "1",
		"-m", "1",
		"-W", "5",
		m.serverAddr,
	}

	//nolint:noctx,gosec
	cmd := exec.Command("turnutils_uclient", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.errChan <- fmt.Errorf("failed to create stdout pipe: %w", err)

		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.errChan <- fmt.Errorf("failed to create stderr pipe: %w", err)

		return
	}

	if err := cmd.Start(); err != nil {
		m.errChan <- fmt.Errorf("failed to start turnutils_uclient: %w (is coturn installed?)", err)

		return
	}

	// Look for success indicators in coturn client output
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "success") || strings.Contains(line, "allocate msg sent") {
				m.messageReceived <- "success"
			}
		}
	}()

	_ = cmd.Wait()
	m.clientDone <- nil
	close(m.clientDone)
}

//nolint:varnamelen
func serverCoturnIPv6(m *testmgr) {
	go func() {
		m.serverMutex.Lock()
		defer m.serverMutex.Unlock()

		host, portStr, err := net.SplitHostPort(m.serverAddr)
		if err != nil {
			m.errChan <- fmt.Errorf("failed to parse serverAddr: %w", err)

			return
		}

		// Coturn requires explicit dual-stack configuration to properly handle IPv6.
		// Both IPv4 (0.0.0.0) and IPv6 addresses must be specified separately.
		// See: https://github.com/coturn/coturn/issues/1294
		config := fmt.Sprintf(`
listening-port=%s
listening-ip=0.0.0.0
listening-ip=%s
relay-ip=%s
external-ip=%s
user=%s:%s
realm=%s
lt-cred-mech
fingerprint
no-tls
no-dtls
log-file=stdout
verbose
`, portStr, host, host, host, m.username, m.password, m.realm)

		configFile := fmt.Sprintf("/tmp/turnserver-%s.conf", m.serverAddr)
		err = os.WriteFile(configFile, []byte(config), 0o600)
		if err != nil {
			m.errChan <- err

			return
		}
		defer func() {
			_ = os.Remove(configFile)
		}()

		//nolint:noctx,gosec
		cmd := exec.Command("turnserver", "-c", configFile)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard

		if err := cmd.Start(); err != nil {
			m.errChan <- fmt.Errorf("failed to start coturn: %w (is coturn installed?)", err)

			return
		}

		time.Sleep(coturnStartupDelay)

		m.serverReady <- struct{}{}

		<-m.ctx.Done()

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()

		time.Sleep(100 * time.Millisecond)

		m.serverDone <- nil
		close(m.serverDone)
	}()
}

//nolint:varnamelen
func clientCoturnIPv6(m *testmgr) {
	select {
	case <-m.serverReady:
	case <-time.After(time.Second * 5):
		m.errChan <- errServerTimeout

		return
	}

	m.clientMutex.Lock()
	defer m.clientMutex.Unlock()

	host, portStr, err := net.SplitHostPort(m.serverAddr)
	if err != nil {
		m.errChan <- err

		return
	}

	args := []string{
		// "-v",
		"-y",
		"-x",
		"-u", m.username,
		"-w", m.password,
		"-r", m.realm,
		"-p", portStr,
		"-n", "1",
		"-m", "1",
		"-W", "5",
		host,
	}

	//nolint:noctx,gosec
	cmd := exec.Command("turnutils_uclient", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.errChan <- fmt.Errorf("failed to create stdout pipe: %w", err)

		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.errChan <- fmt.Errorf("failed to create stderr pipe: %w", err)

		return
	}

	if err := cmd.Start(); err != nil {
		m.errChan <- fmt.Errorf("failed to start turnutils_uclient: %w (is coturn installed?)", err)

		return
	}

	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "success") || strings.Contains(line, "allocate msg sent") {
				m.messageReceived <- "success"
			}
		}
	}()

	_ = cmd.Wait()
	m.clientDone <- nil
	close(m.clientDone)
}

func TestPionCoturnE2EClientServer(t *testing.T) {
	t.Parallel()
	t.Run("CoturnServer", func(t *testing.T) {
		t.Parallel()
		testPionE2ESimple(t, serverCoturn, clientPion)
	})
	t.Run("CoturnClient", func(t *testing.T) {
		t.Parallel()
		testPionE2ESimple(t, serverPion, clientCoturn)
	})
}

func TestPionCoturnE2EClientServerIPv6(t *testing.T) {
	t.Parallel()
	t.Run("CoturnServerIPv6", func(t *testing.T) {
		t.Parallel()
		testPionE2ESimpleIPv6(t, serverCoturnIPv6, clientPionIPv6)
	})
	t.Run("CoturnClientIPv6", func(t *testing.T) {
		t.Parallel()
		testPionE2ESimpleIPv6(t, serverPionIPv6, clientCoturnIPv6)
	})
}
