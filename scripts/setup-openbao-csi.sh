#!/bin/bash
set -e

echo "Setting up OpenBao CSI Provider integration..."

echo ""
echo "Step 1: Checking if CSI Secret Store Driver is installed..."
if ! kubectl get csidriver secrets-store.csi.k8s.io >/dev/null 2>&1; then
    echo "CSI Secret Store Driver not found. Installing..."
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/v1.4.0/deploy/secrets-store-csi-driver.yaml
    echo "Waiting for CSI driver to be ready..."
    kubectl wait --for=condition=ready pod -l app=secrets-store-csi-driver -n kube-system --timeout=120s
else
    echo "CSI Secret Store Driver is already installed"
fi

echo ""
echo "Step 2: Checking if OpenBao CSI Provider is installed..."
if ! kubectl get daemonset -n kube-system -l app=openbao-csi-provider >/dev/null 2>&1; then
    echo "OpenBao CSI Provider not found. Installing..."
    kubectl apply -f https://raw.githubusercontent.com/openbao/openbao-csi-provider/main/deploy/install.yaml
    echo "Waiting for CSI provider to be ready..."
    kubectl wait --for=condition=ready pod -l app=openbao-csi-provider -n kube-system --timeout=120s
else
    echo "OpenBao CSI Provider is already installed"
fi

echo ""
echo "Step 3: Extracting OpenBao root token..."
OPENBAO_NAMESPACE="openbao"
podName=$(kubectl get pods -n "${OPENBAO_NAMESPACE}" -l app=openbao -o jsonpath='{.items[0].metadata.name}')
if [ -z "$podName" ]; then
    echo "Error: OpenBao pod not found. Please deploy OpenBao first."
    exit 1
fi

OPENBAO_TOKEN=$(kubectl logs -n "${OPENBAO_NAMESPACE}" "${podName}" 2>/dev/null | grep "Root Token:" | awk '{print $NF}' | head -1)
if [ -z "$OPENBAO_TOKEN" ]; then
    echo "Error: Could not find root token in OpenBao logs"
    exit 1
fi
echo "Found root token"

echo ""
echo "Step 4: Creating OpenBao token secret..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: openbao-token
  namespace: otel-demo
type: Opaque
stringData:
  token: "${OPENBAO_TOKEN}"
EOF

echo ""
echo "Step 5: Applying RBAC configuration..."
kubectl apply -f kubectl/openbao-csi-rbac.yaml

echo ""
echo "Step 6: Applying SecretProviderClass..."
kubectl apply -f kubectl/openbao-csi-secretproviderclass.yaml

echo ""
echo "Step 7: Updating otelcol1 deployment with CSI volumes..."
kubectl apply -f kubectl/otelcol1-with-csi.yaml

echo ""
echo "Waiting for deployment to be ready..."
kubectl rollout status deployment/otelcol1 -n otel-demo --timeout=120s

echo ""
echo "Step 8: Verifying CSI integration..."
sleep 5
if kubectl get secret otelcol1-certs -n otel-demo >/dev/null 2>&1; then
    echo "Success! Secret 'otelcol1-certs' was created by CSI provider"
    kubectl get secret otelcol1-certs -n otel-demo -o jsonpath='{.data}' | jq -r 'keys[]' 2>/dev/null || echo "Secret exists"
else
    echo "Warning: Secret not yet created. It will be created when the pod starts."
fi

echo ""
echo "Done! OpenBao CSI Provider integration is complete."
echo ""
echo "Certificates are now automatically mounted via CSI volumes:"
echo "  - File mount: /mnt/secrets-store/"
echo "  - K8s Secret: otelcol1-certs (synced automatically)"
