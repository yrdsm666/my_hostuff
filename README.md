# my-hotstuff

`my-hotstuff` is a simple implementation of hotstuff consensus protocol. The code was modified from [wjbbig/go-hotstuff](https://github.com/wjbbig/go-hotstuff).

## Run the example

First, run `scripts/generate_keys.sh` to generate threshold keys, then run the following commands in each of the four terminals to start four hotstuff servers:

```
../server/hotstuff -id 1 -type event-driven

../server/hotstuff -id 2 -type event-driven

../server/hotstuff -id 3 -type event-driven

../server/hotstuff -id 4 -type event-driven
```

There are three kinds of networks you can choose to use, they are `basic`, `chained` and `event-driven`. You can use the flag `-type` to select which network. There is a simple client where you can find in `cmd/hotstuffclient`, or you can run `scripts/run_client.sh` to start it.

Note that the default keystore path is `/home/lmh/Desktop/consensus_code/keys-go-hotstuff/keys` and the default datastore path is `/home/lmh/Desktop/consensus_code/keys-go-hotstuff/dbfile`, so you need to ensure that these two paths have been created before running the program.

## Reference

M. Yin, D. Malkhi, M. K. Reiter, G. Golan Gueta, and I. Abraham, “HotStuff: BFT Consensus in the Lens of Blockchain,” Mar 2018.