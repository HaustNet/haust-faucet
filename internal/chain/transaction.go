package chain

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	log "github.com/sirupsen/logrus"
)

type TxBuilder interface {
	Sender() common.Address
	Transfer(ctx context.Context, to string, value *big.Int) (common.Hash, error)
}

type TxBuild struct {
	client               bind.ContractTransactor
	privateKey           *ecdsa.PrivateKey
	signer               types.Signer
	fromAddress          common.Address
	nonce                uint64
	supportsEIP1559      bool
	nonceRefreshEvery    uint64
	nonceRefreshInterval time.Duration
	lastRefreshTime      time.Time
	nonceMu              sync.Mutex
}

func NewTxBuilder(provider string, privateKey *ecdsa.PrivateKey, chainID *big.Int) (TxBuilder, error) {
	client, err := ethclient.Dial(provider)
	if err != nil {
		return nil, err
	}

	if chainID == nil {
		chainID, err = client.ChainID(context.Background())
		if err != nil {
			return nil, err
		}
	}

	supportsEIP1559, err := checkEIP1559Support(client)
	if err != nil {
		return nil, err
	}

	txBuilder := &TxBuild{
		client:               client,
		privateKey:           privateKey,
		signer:               types.NewLondonSigner(chainID),
		fromAddress:          crypto.PubkeyToAddress(privateKey.PublicKey),
		supportsEIP1559:      supportsEIP1559,
		lastRefreshTime:      time.Time{},
		nonceMu:              sync.Mutex{},
		nonceRefreshInterval: time.Minute * 1,
		nonceRefreshEvery:    100,
	}

	return txBuilder, nil
}

func (b *TxBuild) Sender() common.Address {
	return b.fromAddress
}

func (b *TxBuild) Transfer(ctx context.Context, to string, value *big.Int) (common.Hash, error) {
	gasLimit := uint64(21000)
	gasPrice, err := b.client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	nonce, err := b.getNextNonce(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	toAddress := common.HexToAddress(to)
	unsignedTx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Value:    value,
		Gas:      gasLimit,
		GasPrice: gasPrice,
	})

	signedTx, err := types.SignTx(unsignedTx, b.signer, b.privateKey)
	if err != nil {
		return common.Hash{}, err
	}

	if err = b.client.SendTransaction(ctx, signedTx); err != nil {
		log.Error("failed to send tx", "tx hash", signedTx.Hash().String(), "err", err)
		return common.Hash{}, err
	}

	log.Infof("sent tx %s to %s", signedTx.Hash(), to)

	return signedTx.Hash(), nil
}

func (b *TxBuild) getAndIncrementNonce() uint64 {
	return atomic.AddUint64(&b.nonce, 1) - 1
}

func (b *TxBuild) getNextNonce(ctx context.Context) (uint64, error) {
	b.nonceMu.Lock()
	defer b.nonceMu.Unlock()
	b.nonce++
	// fetch from RPC every n txs, or after refresh interval - whichever is hit first
	if time.Since(b.lastRefreshTime) > b.nonceRefreshInterval || b.nonce%b.nonceRefreshEvery == 0 {
		n, err := b.client.PendingNonceAt(ctx, b.fromAddress)
		if err != nil {
			return 0, err
		}
		b.nonce = n
		b.lastRefreshTime = time.Now()
	}
	nonce := b.nonce
	return nonce, nil
}

func checkEIP1559Support(client *ethclient.Client) (bool, error) {
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return false, err
	}

	return header.BaseFee != nil && header.BaseFee.Cmp(big.NewInt(0)) > 0, nil
}
