all:
	echo 'Provide a target: pow clean'

vendor:
	gb vendor fetch github.com/boltdb/bolt

fmt:
	find src/ -name '*.go' -exec go fmt {} ';'

build: fmt
	gb build all

pow: build
	./bin/pow

test:
	gb test -v

clean:
	rm -rf bin/ pkg/

.PHONY: pow
