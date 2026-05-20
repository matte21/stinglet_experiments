STAGING_LIBS=(
    k8s.io/api
    k8s.io/apimachinery
    k8s.io/client-go
    k8s.io/apiserver
    k8s.io/component-base
    k8s.io/component-helpers
    k8s.io/controller-manager
    k8s.io/apiextensions-apiserver
    k8s.io/dynamic-resource-allocation
    k8s.io/cloud-provider
    k8s.io/csi-translation-lib
)

for lib in "${STAGING_LIBS[@]}"; do
    name="${lib#k8s.io/}"
    go mod edit "-replace=${lib}=/home/molivi/k8s/staging/src/${lib}"
done
