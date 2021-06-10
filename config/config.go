package config

import (
	"dfile-secondary-node/account"
	blockchainprovider "dfile-secondary-node/blockchain_provider"
	"log"

	"dfile-secondary-node/shared"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type SecondaryNodeConfig struct {
	HTTPPort         string `json:"portHTTP"`
	Address          string `json:"publicAddress"`
	StorageLimit     int    `json:"storageLimit"`
	UsedStorageSpace int64  `json:"usedStorageSpace"`
}

var fullyReservedIPs = map[string]bool{
	"0":   true,
	"10":  true,
	"127": true,
}

var partiallyReservedIPs = map[string]int{
	"172": 31,
	"192": 168,
}

func Create(address, password string) (SecondaryNodeConfig, error) {

	dFileConf := SecondaryNodeConfig{}

	if address == "" {
		fmt.Println("Please select one of account addresses")
		accounts := account.List()

		for i, a := range accounts {
			fmt.Println(i+1, a)
		}

		for {
			accountAddress, err := shared.ReadFromConsole()
			if err != nil {
				return dFileConf, err
			}

			addressMatches := shared.ContainsAccount(accounts, accountAddress)

			if !addressMatches {
				fmt.Println("Account is incorrect, try again")
				continue
			}

			dFileConf.Address = accountAddress
			address = accountAddress
			break
		}
	} else {
		dFileConf.Address = address
		fmt.Println("Now, a config file creation is needed.")
	}

	pathToConfig := filepath.Join(shared.AccsDirPath, address, shared.ConfDirName)

	regNum := regexp.MustCompile(("[0-9]+"))

	for {
		fmt.Println("Please enter disk space for usage in GB (should be positive number)")

		availableSpace := shared.GetAvailableSpace(pathToConfig)
		fmt.Println("Available space:", availableSpace, "GB")
		space, err := shared.ReadFromConsole()
		if err != nil {
			return dFileConf, err
		}

		match := regNum.MatchString(space)

		if !match {
			fmt.Println("Value is incorrect, please try again")
			continue
		}

		intSpace, err := strconv.Atoi(space)
		if err != nil {
			fmt.Println("Value is incorrect, please try again")
			continue
		}

		if intSpace < 0 || intSpace >= availableSpace {
			fmt.Println("Passed value is out of avaliable space range, please try again")
			continue
		}

		dFileConf.StorageLimit = intSpace
		break
	}

	regIp := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

	fmt.Println("Please enter your public IP address. Remember if you don't have a static ip address it may change")
	fmt.Println("After router reset ip address info update may be needed")
	fmt.Println("You can check your public ip address by using various online services")

	var splittedAddr []string

	for {

		ipAddr, err := shared.ReadFromConsole()
		if err != nil {
			return dFileConf, err
		}

		match := regIp.MatchString(ipAddr)

		if !match {
			fmt.Println("Value is incorrect, please try again")
			continue
		}

		splittedAddr = strings.Split(ipAddr, ".")

		if fullyReservedIPs[splittedAddr[0]] {
			fmt.Println("Address", ipAddr, "can't be used as a public ip address")
			continue
		}

		reservedSecAddrPart, isReserved := partiallyReservedIPs[splittedAddr[0]]

		if isReserved {
			secondAddrPart, err := strconv.Atoi(splittedAddr[1])
			if err != nil {
				return dFileConf, err
			}

			if secondAddrPart <= reservedSecAddrPart {
				fmt.Println("Address", ipAddr, "can't be used as a public ip address")
				continue
			}
		}

		break
	}

	regPort := regexp.MustCompile("[0-9]+|")

	for {
		fmt.Println("Enter http port number (value from 49152 to 65535) or press enter to use default port number 55050")

		httpPort, err := shared.ReadFromConsole()
		if err != nil {
			return dFileConf, err
		}

		if httpPort == "" {
			dFileConf.HTTPPort = fmt.Sprint(55050)
			break
		}

		match := regPort.MatchString(httpPort)
		if !match {
			fmt.Println("Value is incorrect, please try again")
			continue

		}

		intHttpPort, err := strconv.Atoi(httpPort)
		if err != nil {
			fmt.Println("Value is incorrect, please try again")
			continue
		}

		if intHttpPort < 49152 || intHttpPort > 65535 {
			fmt.Println("Value is incorrect, please try again")
			continue
		}

		dFileConf.HTTPPort = fmt.Sprint(intHttpPort)
		break
	}

	err := blockchainprovider.RegisterNode(address, password, splittedAddr, dFileConf.HTTPPort)
	if err != nil {
		log.Fatal("Couldn't register node in network")
	}

	confFile, err := os.Create(filepath.Join(pathToConfig, "config.json"))
	if err != nil {
		return dFileConf, err
	}
	defer confFile.Close()

	confJSON, err := json.Marshal(dFileConf)
	if err != nil {
		return dFileConf, err
	}

	_, err = confFile.Write(confJSON)
	if err != nil {
		return dFileConf, err
	}

	confFile.Sync()

	return dFileConf, nil
}
