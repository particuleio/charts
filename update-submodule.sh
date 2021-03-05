#! /bin/sh

git fetch istio master && git subtree pull --prefix istio istio master --squash
git fetch aws-ebs-csi-driver master && git subtree pull --prefix aws-ebs-csi-driver aws-ebs-csi-driver master --squash
