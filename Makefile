SUITE_DIR := dectest/testdata/suite

.PHONY: test fuzz testdata suite clean

test:
	go test ./...

fuzz:
	go test -run=- -fuzz=FuzzAddMulExact -fuzztime=30s .

bench:
	go test -run=- -bench=. -benchmem .

# Download the full IBM General Decimal Arithmetic test suite.
testdata:
	@mkdir -p $(SUITE_DIR)
	curl -fsSL https://speleotrove.com/decimal/dectest.zip -o /tmp/dectest.zip
	cd $(SUITE_DIR) && unzip -oq /tmp/dectest.zip
	@echo "Fetched. Run: make suite"

# Run the full suite (after `make testdata`).
suite:
	cd dectest && DECTEST_DIR=testdata/suite go test -run TestSuite -v

clean:
	rm -rf $(SUITE_DIR) /tmp/dectest.zip
