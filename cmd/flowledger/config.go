package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

const (
	defaultSepoliaRPC = "https://ethereum-sepolia-rpc.publicnode.com"
	defaultWalletDir  = "./data/wallet"
	defaultHTTPHost   = "127.0.0.1"
	defaultHTTPPort   = 8090
	defaultSolanaRPC  = "https://api.devnet.solana.com"
	defaultTronRPC    = "https://api.shasta.trongrid.io"
)

// networkDefinition lists the supported networks. Environment settings are applied below.
type networkDefinition struct {
	slug       string
	envPrefix  string
	defaultRPC string
	chainID    int64
}

var networkDefinitions = [...]networkDefinition{
	{slug: "arbitrum", envPrefix: "ARBITRUM_SEPOLIA", defaultRPC: "https://sepolia-rollup.arbitrum.io/rpc", chainID: 421614},
	{slug: "base", envPrefix: "BASE_SEPOLIA", defaultRPC: "https://sepolia.base.org", chainID: 84532},
	{slug: "optimism", envPrefix: "OP_SEPOLIA", defaultRPC: "https://sepolia.optimism.io", chainID: 11155420},
	{slug: "polygon", envPrefix: "POLYGON_AMOY", defaultRPC: "https://polygon-amoy.drpc.org", chainID: 80002},
}

// networkRuntimeConfig holds the checked settings used to start each network service.
type networkRuntimeConfig struct {
	slug    string
	chainID int64
	rpcURLs []string
}

type runtimeConfig struct {
	sepoliaRPCURLs []string
	httpHost       string
	httpPort       int
	accessToken    string
	walletDir      string
	networks       []networkRuntimeConfig
	solanaRPC      string
	tronRPC        string
	tronAPIKey     string
	faucetPath     string
}

// loadConfig reads and checks environment settings without opening wallets or calling RPCs.
func loadConfig(getenv func(string) string) (runtimeConfig, error) {
	portText := cmp.Or(getenv("PORT"), strconv.Itoa(defaultHTTPPort))
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return runtimeConfig{}, errors.New("PORT 必須是 1–65535 的整數")
	}

	host := cmp.Or(getenv("HTTP_HOST"), defaultHTTPHost)
	if host != "127.0.0.1" && host != "0.0.0.0" {
		return runtimeConfig{}, errors.New("HTTP_HOST 僅允許 127.0.0.1 或 0.0.0.0")
	}

	accessToken := getenv("WALLET_ACCESS_TOKEN")
	if accessToken != "" && len(accessToken) < 32 {
		return runtimeConfig{}, errors.New("若設定 WALLET_ACCESS_TOKEN，必須至少 32 字元")
	}

	sepoliaRPC := cmp.Or(getenv("SEPOLIA_RPC_URL"), defaultSepoliaRPC)
	config := runtimeConfig{
		sepoliaRPCURLs: rpcURLs(sepoliaRPC, getenv("SEPOLIA_RPC_FALLBACK_URLS")),
		httpHost:       host,
		httpPort:       port,
		accessToken:    accessToken,
		walletDir:      cmp.Or(getenv("WALLET_DIR"), defaultWalletDir),
		solanaRPC:      cmp.Or(getenv("SOLANA_DEVNET_RPC_URL"), defaultSolanaRPC),
		tronRPC:        cmp.Or(getenv("TRON_SHASTA_RPC_URL"), defaultTronRPC),
		tronAPIKey:     getenv("TRON_SHASTA_API_KEY"),
		faucetPath:     getenv("TEST_FAUCET_CONFIG"),
		networks:       make([]networkRuntimeConfig, 0, len(networkDefinitions)),
	}
	for _, definition := range networkDefinitions {
		endpoint := cmp.Or(getenv(definition.envPrefix+"_RPC_URL"), definition.defaultRPC)
		config.networks = append(config.networks, networkRuntimeConfig{
			slug:    definition.slug,
			chainID: definition.chainID,
			rpcURLs: rpcURLs(endpoint, getenv(definition.envPrefix+"_RPC_FALLBACK_URLS")),
		})
	}

	return config, nil
}

func rpcURLs(primary, fallback string) []string {
	urls := []string{primary}
	if fallback == "" {
		return urls
	}
	for endpoint := range strings.SplitSeq(fallback, ",") {
		urls = append(urls, strings.TrimSpace(endpoint))
	}
	return urls
}

// faucetConfig matches the faucet JSON file. Startup checks it before loading accounts.
type faucetConfig struct {
	AccountID    string `json:"accountId"`
	Password     string `json:"password"`
	TronDir      string `json:"tronDir"`
	TronPassword string `json:"tronPassword"`
}

func readFaucetConfig(filename string) (faucetConfig, error) {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return faucetConfig{}, errors.New("TEST_FAUCET_CONFIG 必須是僅擁有者可讀寫的設定檔（0600）")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return faucetConfig{}, errors.New("無法讀取測試幣發放設定")
	}
	var config faucetConfig
	if err := json.Unmarshal(data, &config); err != nil || config.AccountID == "" || config.Password == "" {
		return faucetConfig{}, errors.New("測試幣發放設定缺少專用 accountId 或 password")
	}
	return config, nil
}
