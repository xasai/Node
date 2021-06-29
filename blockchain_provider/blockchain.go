package blockchainprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	abiPOS "dfile-secondary-node/POS_abi"
	nodeApi "dfile-secondary-node/node_abi"
	"dfile-secondary-node/shared"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type StorageInfo struct {
	Nonce        string     `json:"nonce"`
	SignedFsRoot string     `json:"signedFsRoot"`
	Tree         [][][]byte `json:"tree"`
}

const eightKB = 8192
const NFT = "0xBfAfdaE6B77a02A4684D39D1528c873961528342"

//const ethClientAddr = "https://kovan.infura.io/v3/a4a45777ca65485d983c278291e322f2"

const ethClientAddr = "https://kovan.infura.io/v3/6433ee0efa38494a85541b00cd377c5f"

func RegisterNode(ctx context.Context, address, password string, ip []string, port string) error {
	const logInfo = "blockchainprovider.RegisterNode->"
	ipAddr := [4]uint8{}

	for i, v := range ip {
		intIPPart, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
		}

		ipAddr[i] = uint8(intIPPart)
	}

	intPort, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	nodeAddr, err := shared.DecryptNodeAddr()
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	client, err := ethclient.Dial(ethClientAddr)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	defer client.Close()

	blockNum, err := client.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	balance, err := client.BalanceAt(ctx, common.HexToAddress(address), big.NewInt(int64(blockNum-1)))
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	balanceIsInsufficient := balance.Cmp(big.NewInt(200000000000000)) == -1

	if balanceIsInsufficient {
		fmt.Println("Your account has insufficient funds for registering in net. Balance:", balance, "wei")
		fmt.Println("Please top up your balance")
		os.Exit(0)
	}

	node, err := nodeApi.NewNodeNft(common.HexToAddress(NFT), client)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	opts, _, err := initTrxOpts(ctx, client, nodeAddr, password)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	_, err = node.CreateNode(opts, ipAddr, uint16(intPort))
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	return nil
}

// ====================================================================================

func GetNodeInfoByID() (nodeApi.SimpleMetaDataDeNetNode, error) {
	const logInfo = "blockchainprovider.GetNodeInfoByID->"
	var nodeInfo nodeApi.SimpleMetaDataDeNetNode

	client, err := ethclient.Dial(ethClientAddr)
	if err != nil {
		return nodeInfo, fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	defer client.Close()

	node, err := nodeApi.NewNodeNft(common.HexToAddress(NFT), client)
	if err != nil {
		return nodeInfo, fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	nodeInfo, err = node.GetNodeById(&bind.CallOpts{}, big.NewInt(2))
	if err != nil {
		return nodeInfo, fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	return nodeInfo, nil
}

// ====================================================================================

func UpdateNodeInfo(ctx context.Context, nodeAddr common.Address, password, newPort string, newIP []string) error {
	const logInfo = "blockchainprovider.UpdateNodeInfo->"
	ipInfo := [4]uint8{}

	for i, v := range newIP {
		intPart, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
		}

		ipInfo[i] = uint8(intPart)
	}

	intPort, err := strconv.Atoi(newPort)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	client, err := ethclient.Dial(ethClientAddr)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	defer client.Close()

	node, err := nodeApi.NewNodeNft(common.HexToAddress(NFT), client)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	opts, _, err := initTrxOpts(ctx, client, nodeAddr, password)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	_, err = node.UpdateNode(opts, big.NewInt(2), ipInfo, uint16(intPort))
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	return nil
}

// ====================================================================================

func StartMining(password string) {
	const logInfo = "blockchainprovider.StartMining->"
	nodeAddr, err := shared.DecryptNodeAddr()
	if err != nil {
		shared.LogError(logInfo, shared.GetDetailedError(err))
	}

	pathToAccStorage := filepath.Join(shared.AccsDirPath, nodeAddr.String(), shared.StorageDirName)

	regAddr := regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	regFileName := regexp.MustCompile("[0-9A-Za-z_]")

	client, err := ethclient.Dial(ethClientAddr)
	if err != nil {
		shared.LogError(logInfo, shared.GetDetailedError(err))
	}
	defer client.Close()

	tokenAddress := common.HexToAddress("0x2E8630780A231E8bCf12Ba1172bEB9055deEBF8B")
	instance, err := abiPOS.NewStore(tokenAddress, client)
	if err != nil {
		shared.LogError(logInfo, shared.GetDetailedError(err))
	}

	for {
		fmt.Println("Sleeping...")
		time.Sleep(time.Minute * 10)
		storageProviderAddresses := []string{}
		err = filepath.WalkDir(pathToAccStorage,
			func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				if regAddr.MatchString(info.Name()) {
					storageProviderAddresses = append(storageProviderAddresses, info.Name())
				}

				return nil
			})

		if err != nil {
			shared.LogError(logInfo, shared.GetDetailedError(err))
		}

		if len(storageProviderAddresses) == 0 {
			continue
		}

		ctx, _ := context.WithTimeout(context.Background(), time.Minute*1)

		blockNum, err := client.BlockNumber(ctx)
		if err != nil {
			shared.LogError(logInfo, shared.GetDetailedError(err))
			continue
		}

		nodeBalance, err := client.BalanceAt(ctx, nodeAddr, big.NewInt(int64(blockNum-1)))
		if err != nil {
			shared.LogError(logInfo, shared.GetDetailedError(err))
			continue
		}

		nodeBalanceIsLow := nodeBalance.Cmp(big.NewInt(1500000000000000)) == -1

		if nodeBalanceIsLow {
			fmt.Println("Your account has insufficient funds for paying transaction fee. Balance:", nodeBalance, "wei")
			fmt.Println("Please top up your balance")
			continue
		}

		for _, spAddress := range storageProviderAddresses {

			time.Sleep(time.Minute * 1)

			storageProviderAddr := common.HexToAddress(spAddress)
			_, reward, userDifficulty, err := instance.GetUserRewardInfo(&bind.CallOpts{}, storageProviderAddr) // first value is paymentToken
			if err != nil {
				shared.LogError(logInfo, shared.GetDetailedError(err))
			}

			fmt.Println("reward for", spAddress, "files is", reward) //TODO remove
			fmt.Println("Min reward value:", 350000000000000000)

			rewardisEnough := reward.Cmp(big.NewInt(350000000000000000)) == 1

			if !rewardisEnough {
				continue
			}

			fileNames := []string{}

			pathToStorProviderFiles := filepath.Join(pathToAccStorage, storageProviderAddr.String())

			err = filepath.WalkDir(pathToStorProviderFiles,
				func(path string, info fs.DirEntry, err error) error {
					if err != nil {
						shared.LogError(logInfo, shared.GetDetailedError(err))
					}

					if regFileName.MatchString(info.Name()) && len(info.Name()) == 64 {
						fileNames = append(fileNames, info.Name())

					}

					return nil
				})
			if err != nil {
				shared.LogError(logInfo, shared.GetDetailedError(err))
			}

			pathToFsTree := filepath.Join(shared.AccsDirPath, nodeAddr.String(), shared.StorageDirName, spAddress, "tree.json")

			shared.MU.Lock()
			fileFsTree, err := os.Open(pathToFsTree)
			if err != nil {
				shared.MU.Unlock()
				shared.LogError(logInfo, shared.GetDetailedError(err))
			}

			treeBytes, err := io.ReadAll(fileFsTree)
			if err != nil {
				fileFsTree.Close()
				shared.MU.Unlock()
				shared.LogError(logInfo, shared.GetDetailedError(err))
			}
			fileFsTree.Close()
			shared.MU.Unlock()

			var storageFsStruct StorageInfo

			err = json.Unmarshal(treeBytes, &storageFsStruct)
			if err != nil {
				shared.LogError(logInfo, shared.GetDetailedError(err))
			}

			fsFiles := map[string]bool{}

			for _, hashes := range storageFsStruct.Tree {
				for _, hash := range hashes {
					fsFiles[hex.EncodeToString(hash)] = true
				}
			}

			for _, fileName := range fileNames {

				time.Sleep(time.Minute)

				if !fsFiles[fileName] {
					fmt.Println("removing file", fileName)
					err = os.Remove(filepath.Join(pathToStorProviderFiles, fileName))
					if err != nil {
						shared.LogError(logInfo, shared.GetDetailedError(err))
					}

					fsFiles = nil
					continue
				}

				fsFiles = nil

				storedFile, err := os.Open(filepath.Join(pathToStorProviderFiles, fileName))
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				storedFileBytes, err := io.ReadAll(storedFile)
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				storedFile.Close()

				ctx, _ := context.WithTimeout(context.Background(), time.Minute*1)

				blockNum, err := client.BlockNumber(ctx)
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
					break
				}

				blockHash, err := instance.GetBlockHash(&bind.CallOpts{}, uint32(blockNum-1))
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				fileBytesAddrBlockHash := append(storedFileBytes, nodeAddr.Bytes()...)
				fileBytesAddrBlockHash = append(fileBytesAddrBlockHash, blockHash[:]...)

				hashedFileAddrBlock := sha256.Sum256(fileBytesAddrBlockHash)

				stringFileAddrBlock := hex.EncodeToString(hashedFileAddrBlock[:])

				stringFileAddrBlock = strings.TrimLeft(stringFileAddrBlock, "0")

				decodedBigInt, err := hexutil.DecodeBig("0x" + stringFileAddrBlock)
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				baseDfficulty, err := instance.BaseDifficulty(&bind.CallOpts{})
				if err != nil {
					shared.LogError(logInfo, shared.GetDetailedError(err))
				}

				remainder := decodedBigInt.Rem(decodedBigInt, baseDfficulty)

				compareResultIsLessUserDifficulty := remainder.CmpAbs(userDifficulty) == -1

				if compareResultIsLessUserDifficulty {
					fmt.Println("checking file:", fileName)
					fmt.Println("Sending proof of", fileName, "for reward:", reward)
					err := sendProof(ctx, client, password, storedFileBytes, nodeAddr, spAddress)
					if err != nil {
						shared.LogError(logInfo, shared.GetDetailedError(err))
						break
					}
				}
			}
		}
	}
}

// ====================================================================================

func initTrxOpts(ctx context.Context, client *ethclient.Client, nodeAddr common.Address, password string) (*bind.TransactOpts, uint64, error) {
	const logInfo = "blockchainprovider.initTrxOpts->"

	blockNum, err := client.BlockNumber(ctx)
	if err != nil {
		shared.LogError(logInfo, shared.GetDetailedError(err))
	}

	transactNonce, err := client.NonceAt(ctx, nodeAddr, big.NewInt(int64(blockNum-1)))
	if err != nil {
		return nil, 0, fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	chnID, err := client.ChainID(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	opts := &bind.TransactOpts{
		From:  nodeAddr,
		Nonce: big.NewInt(int64(transactNonce)),
		Signer: func(a common.Address, t *types.Transaction) (*types.Transaction, error) {
			ks := keystore.NewKeyStore(shared.AccsDirPath, keystore.StandardScryptN, keystore.StandardScryptP)
			acs := ks.Accounts()
			for _, ac := range acs {
				if ac.Address == a {
					ks := keystore.NewKeyStore(shared.AccsDirPath, keystore.StandardScryptN, keystore.StandardScryptP)
					err := ks.TimedUnlock(ac, password, 1)
					if err != nil {
						return t, err
					}
					return ks.SignTx(ac, t, chnID)
				}
			}
			return t, nil
		},
		Value:    big.NewInt(0),
		GasPrice: big.NewInt(1000000000),
		GasLimit: 1000000,
		Context:  ctx,
		NoSend:   false,
	}

	return opts, blockNum, nil
}

// ====================================================================================

func sendProof(ctx context.Context, client *ethclient.Client, password string, fileBytes []byte, nodeAddr common.Address, spAddress string) error {
	const logInfo = "blockchainprovider.sendProof->"
	pathToFsTree := filepath.Join(shared.AccsDirPath, nodeAddr.String(), shared.StorageDirName, spAddress, "tree.json")

	shared.MU.Lock()
	fileFsTree, err := os.Open(pathToFsTree)
	if err != nil {
		shared.MU.Unlock()
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	treeBytes, err := io.ReadAll(fileFsTree)
	if err != nil {
		fileFsTree.Close()
		shared.MU.Unlock()
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}
	fileFsTree.Close()
	shared.MU.Unlock()

	var storageFsStruct StorageInfo

	err = json.Unmarshal(treeBytes, &storageFsStruct)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	eightKBHashes := []string{}

	bytesToProve := fileBytes[:eightKB]

	for i := 0; i < len(fileBytes); i += eightKB {
		hSum := sha256.Sum256(fileBytes[i : i+eightKB])
		eightKBHashes = append(eightKBHashes, hex.EncodeToString(hSum[:]))
	}

	_, fileTree, err := shared.CalcRootHash(eightKBHashes)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	hashFileRoot := fileTree[len(fileTree)-1][0]

	treeToFsRoot := [][][]byte{}

	for _, baseHash := range storageFsStruct.Tree[0] {
		diff := bytes.Compare(hashFileRoot, baseHash)
		if diff == 0 {
			treeToFsRoot = append(treeToFsRoot, fileTree[:len(fileTree)-1]...)
			treeToFsRoot = append(treeToFsRoot, storageFsStruct.Tree...)
		}
	}

	proof := makeProof(fileTree[0][0], treeToFsRoot)

	tokenAddress := common.HexToAddress("0x2E8630780A231E8bCf12Ba1172bEB9055deEBF8B")
	instance, err := abiPOS.NewStore(tokenAddress, client)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	signedFSRootHash, err := hex.DecodeString(storageFsStruct.SignedFsRoot)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	opts, blockNum, err := initTrxOpts(ctx, client, nodeAddr, password)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	intNonce, err := strconv.Atoi(storageFsStruct.Nonce)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	_, err = instance.SendProof(opts, common.HexToAddress(spAddress), uint32(blockNum-1), proof[len(proof)-1], uint64(intNonce), signedFSRootHash[:64], bytesToProve, proof)
	if err != nil {
		return fmt.Errorf("%s %w", logInfo, shared.GetDetailedError(err))
	}

	fmt.Println("Got some cash 0_o")

	return nil
}

// ====================================================================================

func getPos(hash []byte, list [][]byte) int {
	for i, v := range list {
		diff := bytes.Compare(v, hash)
		if diff == 0 {
			return i
		}
	}

	return -1
}

// ====================================================================================

// Builds merkle tree proof
func makeProof(start []byte, tree [][][]byte) [][32]byte { // returns slice of 32 bytes array because smart contract awaits this type
	stage := 0
	proof := [][32]byte{}

	var firstNodePosition int
	var secondNodePosition int

	for stage < len(tree) {
		pos := getPos(start, tree[stage])
		if pos == -1 {
			break
		}

		if pos%2 != 0 {
			firstNodePosition = pos - 1
			secondNodePosition = pos
		} else {
			firstNodePosition = pos
			secondNodePosition = pos + 1
		}

		if len(tree[stage]) == 1 {
			root := [32]byte{}
			for i, v := range tree[stage][0] {
				root[i] = v
			}

			proof = append(proof, root)

			return proof
		}

		firstNode := [32]byte{}
		for i, v := range tree[stage][firstNodePosition] {
			firstNode[i] = v
		}

		proof = append(proof, firstNode)

		secondNode := [32]byte{}
		for i, v := range tree[stage][secondNodePosition] {
			secondNode[i] = v
		}

		proof = append(proof, secondNode)

		concatBytes := append(tree[stage][firstNodePosition], tree[stage][secondNodePosition]...)
		hSum := sha256.Sum256(concatBytes)

		start = hSum[:]
		stage++
	}

	return proof
}
