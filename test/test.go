package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"github.com/niclabs/tcrsa"
	"fmt"
	"bytes"
	"encoding/binary"
	"encoding/json"
)

func CreateDocumentHash(msgBytes []byte, meta *tcrsa.KeyMeta) ([]byte, error) {
	bytes := sha256.Sum256(msgBytes)
	documentHash, err := tcrsa.PrepareDocumentHash(meta.PublicKey.Size(), crypto.SHA256, bytes[:])
	if err != nil {
		return nil, err
	}
	return documentHash, nil
}

// TSign create the partial signature of replica
func TSign(documentHash []byte, privateKey *tcrsa.KeyShare, publicKey *tcrsa.KeyMeta) (*tcrsa.SigShare, error) {
	// sign
	partSig, err := privateKey.Sign(documentHash, crypto.SHA256, publicKey)
	if err != nil {
		return nil, err
	}
	// ensure the partial signature is correct
	err = partSig.Verify(documentHash, publicKey)
	if err != nil {
		return nil, err
	}

	return partSig, nil
}


func VerifyPartSig(partSig *tcrsa.SigShare, documentHash []byte, publicKey *tcrsa.KeyMeta) error {
	err := partSig.Verify(documentHash, publicKey)
	if err != nil {
		return err
	}
	return nil
}

func CreateFullSignature(documentHash []byte, partSigs tcrsa.SigShareList, publicKey *tcrsa.KeyMeta) (tcrsa.Signature, error) {
	signature, err := partSigs.Join(documentHash, publicKey)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func TVerify(publicKey *tcrsa.KeyMeta, signature tcrsa.Signature, msgBytes []byte) (bool, error) {
	// get the msg hash
	// documentHash, err := CreateDocumentHash(msgBytes, publicKey)
	// if err != nil {
	// 	return false, err
	// }
	bytes := sha256.Sum256(msgBytes)
	// ensure len(bytes)=32
	err := rsa.VerifyPKCS1v15(publicKey.PublicKey, crypto.SHA256, bytes[:], signature)
	if err != nil {
		return false, err
	}
	return true, nil
}

const size = 2048

func GenerateThresholdKeys(need, all int) (shares tcrsa.KeyShareList, meta *tcrsa.KeyMeta, err error) {
	k := uint16(need)
	l := uint16(all)

	return tcrsa.NewKey(size, k, l, nil)
}

func BytesToInt(bys []byte) int {
	bytebuff := bytes.NewBuffer(bys)
	var data int64
	binary.Read(bytebuff, binary.BigEndian, &data)
	return int(data)
}

func main(){
	fmt.Println("genThresholdKey...")
	keys, meta, err := GenerateThresholdKeys(3, 4)
	if err != nil {
		fmt.Println(err)
	}

	for j:=0;j<10;j++{
		
		cryptoStrBytes := []byte("hello, world" + string(j))
		fmt.Println("str: hello, world" + string(j))

		docHash, err := CreateDocumentHash(cryptoStrBytes, meta)
		if err != nil {
			fmt.Println(err)
		}

		sigList := make(tcrsa.SigShareList, len(keys))
		i := 0
		for i< len(keys) {
			sigList[i], err = TSign(docHash, keys[i], meta)
			if err != nil {
				fmt.Println("TSign error")
				fmt.Println(err)
			}
			err := VerifyPartSig(sigList[i], docHash, meta)
			if err != nil {
				fmt.Println("VerifyPartSig error")
				fmt.Println(err)
			}
		}
		signature, err := CreateFullSignature(docHash, sigList, meta)
		if err != nil {
			fmt.Println("CreateFullSignature error")
			fmt.Println(err)
		}

		_, err = TVerify(meta, signature, cryptoStrBytes)
		if err != nil {
			fmt.Println("TVerify error")
			fmt.Println(len(signature))
			fmt.Println(err)
		}

		marshalData, _ := json.Marshal(signature)
		signatureHash, _ := CreateDocumentHash(marshalData, meta)
		leader := BytesToInt(signatureHash)%4 + 1
		fmt.Println("leader: ",leader)
		fmt.Println("ok")
	}	
}