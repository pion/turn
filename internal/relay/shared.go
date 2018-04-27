package relayServer

import "math/rand"

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func buildTransactionId() []byte {
	transactionID := []byte(randSeq(16))
	transactionID[0] = 33
	transactionID[1] = 18
	transactionID[2] = 164
	transactionID[3] = 66
	return transactionID
}
