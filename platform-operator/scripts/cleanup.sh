helm uninstall kagenti-platform-operator -n kagenti-system
kubectl delete certificate serving-cert -n kagenti-system
kubectl delete secret serving-cert -n kagenti-system --ignore-not-found
kubectl delete certificate metrics-certs -n kagenti-system
kubectl delete secret metrics-certs -n kagenti-system --ignore-not-found
