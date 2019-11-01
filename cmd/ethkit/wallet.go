package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/arcadeum/ethkit/ethwallet"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

func init() {
	wallet := &wallet{}
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "wallet",
		Run:   wallet.Run,
	}

	cmd.Flags().String("file", "", "wallet filename")
	cmd.Flags().Bool("new", false, "create a new wallet and save it to the file")
	cmd.Flags().Bool("print-account", true, "print wallet account address (default)")
	cmd.Flags().Bool("print-mnemonic", false, "print wallet secret mnemonic (danger!)")
	cmd.Flags().Bool("import-mnemonic", false, "import a secret mnemonic")
	// cmd.Flags().String("derivationPath", false, "set derivation path")

	rootCmd.AddCommand(cmd)
}

type wallet struct {
}

type walletKeyFile struct {
	Address common.Address      `json:"address"`
	Path    string              `json:"path"`
	Crypto  keystore.CryptoJSON `json:"crypto"`
	Client  string              `json:"client"`
}

func (c *wallet) Run(cmd *cobra.Command, args []string) {
	fFile, _ := cmd.Flags().GetString("file")
	fCreateNew, _ := cmd.Flags().GetBool("new")
	fPrintAccount, _ := cmd.Flags().GetBool("print-account")
	fPrintMnemonic, _ := cmd.Flags().GetBool("print-mnemonic")
	fImportMnemonic, _ := cmd.Flags().GetBool("import-mnemonic")

	if fFile == "" {
		fmt.Println("error: please pass --file")
		help(cmd)
		return
	}
	if fileExists(fFile) && fCreateNew {
		fmt.Println("error: file already exists, cannot create a new wallet to this path.")
		help(cmd)
		return
	}
	if !fCreateNew && !fPrintMnemonic && !fPrintAccount {
		fmt.Println("error: not enough options provided to ethkit cli.")
		help(cmd)
		return
	}

	// Gen new wallet
	if fCreateNew {
		if err := c.createNew(fFile, fImportMnemonic); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Print mnemonic
	if fPrintMnemonic {
		if err := c.printMnemonic(fFile); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Print acconut address
	if fPrintAccount {
		if err := c.printAccount(fFile); err != nil {
			log.Fatal(err)
		}
		return
	}

}

func (c *wallet) printMnemonic(fFile string) error {
	data, err := ioutil.ReadFile(fFile)
	if err != nil {
		return err
	}
	keyFile := walletKeyFile{}
	err = json.Unmarshal(data, &keyFile)
	if err != nil {
		return err
	}

	pw, err := readSecretInput("Password: ")
	if err != nil {
		return err
	}

	cipherText, err := keystore.DecryptDataV3(keyFile.Crypto, string(pw))
	if err != nil {
		return err
	}

	w, err := ethwallet.NewWalletFromMnemonic(string(cipherText)) // TODO: pass path later
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("")
	fmt.Println("=> PLEASE KEEP THIS SECRET!!! DO NOT SHARE!!")
	fmt.Println("")
	fmt.Println("=> Your Ethereum private mnemonic is:")
	fmt.Println("=>", w.HDNode().Mnemonic())
	fmt.Println("")

	return nil
}

func (c *wallet) printAccount(fFile string) error {
	data, err := ioutil.ReadFile(fFile)
	if err != nil {
		return err
	}
	keyFile := walletKeyFile{}
	err = json.Unmarshal(data, &keyFile)
	if err != nil {
		return err
	}

	pw, err := readSecretInput("Password: ")
	if err != nil {
		return err
	}

	cipherText, err := keystore.DecryptDataV3(keyFile.Crypto, string(pw))
	if err != nil {
		return err
	}

	w, err := ethwallet.NewWalletFromMnemonic(string(cipherText)) // TODO: pass path later
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("")
	fmt.Println("=> Your Ethereum wallet address is:", w.Address().String())
	fmt.Println("")

	return nil
}

func (c *wallet) createNew(fFile string, fImportMnemonic bool) error {
	var err error
	var w *ethwallet.Wallet

	// TODO: allow to change
	derivatonPath := "m/44'/60'/0'/0/0"

	if fImportMnemonic {
		var mnemonic []byte
		mnemonic, err = readPlainInput("Enter your mnemonic to import: ")
		if err != nil {
			return err
		}

		w, err = ethwallet.NewWalletFromMnemonic(strings.TrimSpace(string(mnemonic)), derivatonPath)
	} else {
		w, err = ethwallet.NewWalletFromRandomEntropy(ethwallet.WalletOptions{
			DerivationPath:             derivatonPath,
			RandomWalletEntropyBitSize: ethwallet.EntropyBitSize24WordMnemonic,
		})
	}

	if err != nil {
		return err
	}

	pw, err := readSecretInput("Password: ")
	if err != nil {
		return err
	}
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	fmt.Println("")
	confirmPw, err := readSecretInput("Confirm Password: ")
	if err != nil {
		return err
	}
	if string(pw) != string(confirmPw) {
		return errors.New("passwords do not match")
	}

	cryptoJSON, err := keystore.EncryptDataV3([]byte(w.HDNode().Mnemonic()), pw, keystore.StandardScryptN, keystore.StandardScryptP)
	if err != nil {
		return err
	}

	keyFile := walletKeyFile{
		Address: w.Address(),
		Path:    w.HDNode().DerivationPath().String(),
		Crypto:  cryptoJSON,
		Client:  "ethkit/v0.1", // TODO: do not hardcode
	}

	data, err := json.Marshal(keyFile)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(fFile, data, 0600); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("")
	fmt.Println("=> success! ethkit has generated a new Ethereum wallet for you and saved")
	fmt.Println("=> it in an encrypted+password protected file at:")
	fmt.Println("=> ---")
	fmt.Println("=>", fFile)
	fmt.Println("")
	fmt.Printf("=> to confirm, please run: ./ethkit wallet --file=%s --print-account\n", fFile)
	fmt.Println("")
	fmt.Println("=> Your new Ethereum wallet address is:", w.Address().String())
	fmt.Println("")

	return nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func readSecretInput(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	password, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return nil, err
	}
	return password, nil
}

func readPlainInput(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return []byte(text), nil
}
