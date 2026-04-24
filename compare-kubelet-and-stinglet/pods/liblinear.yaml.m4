apiVersion: v1
kind: Pod
metadata:
  name: liblinear
spec:
  containers:
    - name: liblinear
      image: localhost/mattedocker/liblinear-multicore:v0
      command: ["./liblinear-multicore-train"]
      args: ["-s", "6", "-m", "6", "/kdd12"]
      resources:
        requests:
          cpu: "6"
          memory: "70Gi"
        limits:
          cpu: "6"
          memory: "70Gi"
      volumeMounts:
      - mountPath: /kdd12
        name: dataset
  volumes:
    - name: dataset
      hostPath:
        type: File
        path: HOST_PATH/kdd12
  restartPolicy: Never
