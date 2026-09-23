// SPDX-License-Identifier: AGPL-3.0-only
// newscontainer wraps an original payload using private owner-provided keys.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openpak/news/internal/bcatcontainer"
)

func run() error {
	input := flag.String("in", "", "decoded original payload")
	output := flag.String("out", "", "new container file; must not exist")
	aesFile := flag.String("aes-key-file", "", "private raw title AES key (16/24/32 bytes)")
	signingFile := flag.String("signing-key-file", "", "private OpenPak RSA PEM key")
	index := flag.Uint("secret-index", 0, "BCAT secret index (0..15)")
	flag.Parse()
	if *input == "" || *output == "" || *aesFile == "" || *signingFile == "" || *index > 15 {
		return fmt.Errorf("require -in, -out, -aes-key-file, -signing-key-file and valid secret index")
	}
	plain, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(*aesFile)
	if err != nil {
		return err
	}
	encoded, err := os.ReadFile(*signingFile)
	if err != nil {
		return err
	}
	signing, err := bcatcontainer.ParsePrivateKeyPEM(encoded)
	if err != nil {
		return err
	}
	out, err := bcatcontainer.Encode(plain, key, byte(*index), signing)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(out); err != nil {
		f.Close()
		os.Remove(*output)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(*output)
		return err
	}
	fmt.Printf("Wrote %d-byte BCAT container (%d decoded bytes)\n", len(out), len(plain))
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
