apiVersion: v1
kind: Pod
metadata:
  name: pagerank-12-cpus-fm
  annotations:
    pagerank/far-mem: "512Mi"
spec:
  containers:
    - name: pagerank
      image: localhost/mattedocker/pagerank:v0
      command: ["./pagerank"]
      args: ["-f", "/twitter.sg", "-i1000", "-t1e-4", "-n20"]
      resources:
        requests:
          cpu: "12"
          memory: "12186Mi"
        limits:
          cpu: "12"
          memory: "12186Mi"
      volumeMounts:
      - mountPath: /twitter.sg
        name: dataset
  volumes:
    - name: dataset
      hostPath:
        type: File
        path: HOST_PATH/twitter.sg
  restartPolicy: Never
