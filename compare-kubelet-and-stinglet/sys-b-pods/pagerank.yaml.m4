apiVersion: v1
kind: Pod
metadata:
  name: pagerank
  annotations:
    pagerank/cpus-distribution-over-caches: "l1-spread"
spec:
  containers:
    - name: pagerank
      image: localhost/mattedocker/pagerank:v0
      command: ["./pagerank"]
      args: ["-f", "/twitter.sg", "-i1000", "-t1e-4", "-n20"]
      resources:
        requests:
          cpu: "8"
          memory: "12698Mi"
        limits:
          cpu: "8"
          memory: "12698Mi"
      volumeMounts:
      - mountPath: /twitter.sg
        name: dataset
  volumes:
    - name: dataset
      hostPath:
        type: File
        path: HOST_PATH/twitter.sg
  restartPolicy: Never
