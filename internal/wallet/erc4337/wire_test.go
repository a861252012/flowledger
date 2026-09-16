package erc4337

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestUserOperationRPCBoundaryRoundTripPreservesHashesAndSignature(t *testing.T) {
	t.Parallel()

	original := getOfficialTestOpV06()
	original.Signature = mustHex("0x8df0873ef40e3dccc92d4f723196a448af8cfcb83a53fbb59d041370fcb510093f05cff054024c8bc35b7b49a4647c6ec5c49865ede12847a635434454702e981c")

	rpc := userOperationToRPC(original)
	if rpc.Sender != "0x1111111111111111111111111111111111111111" || rpc.Nonce != "0x0" || rpc.CallGasLimit != "0x186a0" {
		t.Fatalf("unexpected v0.6 RPC conversion: %+v", rpc)
	}

	wireJSON, err := json.Marshal(rpc)
	if err != nil {
		t.Fatal(err)
	}
	var primitiveFields map[string]string
	if err := json.Unmarshal(wireJSON, &primitiveFields); err != nil {
		t.Fatalf("RPC fields must remain primitive strings: %v", err)
	}

	restored, err := userOperationRPCToDomain(rpc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.Signature, original.Signature) {
		t.Fatal("signature changed across RPC boundary")
	}
	for _, entryPoint := range []struct {
		name    string
		address common.Address
	}{
		{name: "v0.6", address: CanonicalEntryPointV06},
		{name: "v0.7", address: CanonicalEntryPointV07},
	} {
		t.Run(entryPoint.name, func(t *testing.T) {
			want, err := GetUserOpHash(original, entryPoint.address, big.NewInt(1))
			if err != nil {
				t.Fatal(err)
			}
			got, err := GetUserOpHash(restored, entryPoint.address, big.NewInt(1))
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("hash changed across RPC boundary: got %s want %s", got, want)
			}
		})
	}
}

func TestUserOperationRPCSenderPreservesLegacyLowercaseJSON(t *testing.T) {
	t.Parallel()

	op := getOfficialTestOpV06()
	op.Sender = common.HexToAddress("0x90F8bf6A479f320ead074411a4B0e7944Ea8c9C1")

	got, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["sender"] != "0x90f8bf6a479f320ead074411a4b0e7944ea8c9c1" {
		t.Fatalf("sender JSON casing changed: %s", fields["sender"])
	}
}

func TestUserOperationUnmarshalFailureIsAtomic(t *testing.T) {
	t.Parallel()

	op := getOfficialTestOpV06()
	before := op.Clone()
	rpc := userOperationToRPC(op)
	rpc.Signature = "0xzz"
	input, err := json.Marshal(rpc)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(input, op); err == nil {
		t.Fatal("malformed signature unexpectedly decoded")
	}
	assertUserOpEqual(t, before, op)
}

func TestPackedUserOperationRPCBoundaryRoundTrip(t *testing.T) {
	t.Parallel()

	packed, err := getOfficialTestOpV06().ToPacked()
	if err != nil {
		t.Fatal(err)
	}
	rpc := packedUserOperationToRPC(packed)
	restored, err := packedUserOperationRPCToDomain(rpc)
	if err != nil {
		t.Fatal(err)
	}
	assertPackedUserOpEqual(t, packed, restored)
}

func TestPackedUserOperationValidatePreservesPackingContract(t *testing.T) {
	packed, err := getOfficialTestOpV06().ToPacked()
	if err != nil {
		t.Fatal(err)
	}
	// Hashing must still support structurally valid operations that execution rejects.
	packed.GasFees, err = PackUint128Pair(big.NewInt(2), big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}
	packed.PaymasterAndData = []byte{1}
	if _, err := PackPackedUserOp(packed); err != nil {
		t.Fatalf("packing contract narrowed: %v", err)
	}
	packed.Nonce = big.NewInt(-1)
	if err := packed.Validate(); !errors.Is(err, ErrNegativeValue) {
		t.Fatalf("negative nonce accepted: %v", err)
	}
}
