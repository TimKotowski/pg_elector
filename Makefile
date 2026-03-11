test:
	@go install gotest.tools/gotestsum@latest

	@mkdir -p tmp/
	@gotestsum \
		--junitfile tmp/test-report.xml \
		--format pkgname-and-test-fails \
		-- \
		-race \
		-coverprofile=tmp/coverage.txt \
		-failfast \
		-shuffle=on \
		-covermode=atomic \
		./...

coverage:
	@go tool cover -html=tmp/coverage.txt

go-version:
	@go version

sqlc-generate:
	cd driver/pgxv5/internal/dbsqlc && sqlc generate
	cd driver/databasesql/internal/dbsqlc && sqlc generate