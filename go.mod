module github.com/binance/zkmerkle-proof-of-solvency

go 1.18

require (
	github.com/aws/aws-sdk-go-v2 v1.17.3
	github.com/aws/aws-sdk-go-v2/config v1.1.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.15.1
	github.com/bnb-chain/zkbnb-smt v0.0.3-0.20221227064653-7422bfd51aa0
	github.com/consensys/gnark v0.7.0
	github.com/consensys/gnark-crypto v0.7.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/shopspring/decimal v1.3.1
	github.com/zeromicro/go-zero v1.4.2
	gorm.io/driver/postgres v1.4.5
	gorm.io/gorm v1.24.2
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.0.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.0.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.1.1 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/ethereum/go-ethereum v1.10.26 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fxamacker/cbor/v2 v2.4.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gocarina/gocsv v0.0.0-20230123225133-763e25b40669 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/hashicorp/golang-lru v0.5.5-0.20221011183528-d4900dc688bf // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.13.0 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.1 // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgtype v1.12.0 // indirect
	github.com/jackc/pgx/v4 v4.17.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.4 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mmcloughlin/addchain v0.4.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.0 // indirect
	github.com/panjf2000/ants/v2 v2.5.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pelletier/go-toml/v2 v2.0.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rs/zerolog v1.26.1 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/otel v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.11.0 // indirect
	go.opentelemetry.io/otel/exporters/zipkin v1.11.0 // indirect
	go.opentelemetry.io/otel/sdk v1.11.0 // indirect
	go.opentelemetry.io/otel/trace v1.11.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/automaxprocs v1.5.1 // indirect
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa // indirect
	golang.org/x/net v0.0.0-20220607020251-c690dde0001d // indirect
	golang.org/x/sys v0.2.0 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220602131408-e326c6e8e9c8 // indirect
	google.golang.org/grpc v1.50.1 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace (
	github.com/consensys/gnark => github.com/bnb-chain/gnark v0.7.1-0.20230203031713-0d81c67d080a
	github.com/consensys/gnark-crypto => github.com/bnb-chain/gnark-crypto v0.7.1-0.20230203031630-7c643ad11891
)
