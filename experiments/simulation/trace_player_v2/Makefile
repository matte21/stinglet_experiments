K8S_DIR = /home/molivi/k8s
BIN_DIR = ./bin

VARIANTS = v1.33.0-std v1.33.0++

all: $(VARIANTS)

$(VARIANTS):
	git -C $(K8S_DIR) checkout $@ && \
	K8S_BRANCH=$$(git -C $(K8S_DIR) rev-parse --abbrev-ref HEAD) && \
	K8S_COMMIT=$$(git -C $(K8S_DIR) rev-parse --short HEAD) && \
	go build \
	  -ldflags="-X main.Version=$${K8S_BRANCH}@$${K8S_COMMIT}" \
	  -o $(BIN_DIR)/sim-$${K8S_BRANCH}

clean:
	rm -f $(BIN_DIR)/sim-*
