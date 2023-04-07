#!/usr/bin/env bash

go build -o ../cmd/hotstuffgenkey/hotstuffgenkey ../cmd/hotstuffgenkey/main.go

../cmd/hotstuffgenkey/hotstuffgenkey -p /home/lmh/Desktop/consensus_code/keys-go-hotstuff/keys -k 3 -l 4