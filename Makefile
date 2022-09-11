.PHONY: genall
genall:
	hack/update-codegen.sh
	hack/update-crdgen.sh

.PHONY: vendor
vendor:
	go mod tidy && go mod vendor

.PHONY: run
run:
	kubectl apply -f deploy
	docker buildx build --push --platform linux/amd64 -t daocloud.io/atsctoo/firefly-controller-manager .
	kubectl rollout restart deployment firefly-controller-manager
	sleep 1
	kubectl get po

.PHONY: local-run
local-run:
	go run cmd/firefly-controller-manager/controller-manager.go \
		--kubeconfig ~/.kube/config \
		--authentication-kubeconfig ~/.kube/config \
		--authorization-kubeconfig ~/.kube/config

.PHONY: run1
run1:
	go run cmd/firefly-karmada-manager/controller-manager.go \
		--karmada-kubeconfig /root/workspaces/firefly/config \
		--authentication-kubeconfig /root/workspaces/firefly/config \
		--authorization-kubeconfig /root/workspaces/firefly/config \
		--firefly-kubeconfig ~/.kube/config
