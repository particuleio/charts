#!/bin/bash

# Copyright 2019 Istio Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)

set -eu

out="${1}"
config="${out}/docker-bake.hcl"
shift

function to_platform_list() {
  image="${1}"
  platforms="${2}"
  # Allow list images verified to work with multi architecture
  # Eventually this should be everything but the testing images (app_sidecar_)
  if [[ "${image}" == base || "${image}" == distroless ]]; then
    # convert CSV to "foo","bar" list
    # shellcheck disable=SC2001
    echo "\"$(echo "${platforms}" | sed 's/,/","/g')\""
  else
    echo '"linux/amd64"'
  fi
}



variants=\"$(for i in ${DOCKER_ALL_VARIANTS}; do echo "\"${i}\""; done | xargs | sed -e 's/ /\", \"/g')\"
cat <<EOF > "${config}"
group "all" {
    targets = [${variants}]
}
EOF

# Generate the top header. This defines a group to build all images for each variant
for variant in ${DOCKER_ALL_VARIANTS}; do
  # Get all images. Transform from `docker.target` to `"target"` as a comma separated list
  images=\"$(for i in "$@"; do
    if ! "${WD}/skip-image.sh" "$i" "$variant"; then echo "\"${i#docker.}-${variant}\""; fi
  done | xargs | sed -e 's/ /\", \"/g')\"
  cat <<EOF >> "${config}"
group "${variant}" {
    targets = [${images}]
}
EOF
done

# For each docker image, define a target to build it
for file in "$@"; do
  for variant in ${DOCKER_ALL_VARIANTS}; do
    image=${file#docker.}
    tag="${TAG}"
    # The default variant has no suffix, others do
    if [[ "${variant}" != "default" ]]; then
      tag+="-${variant}"
    fi

    # Output locally (like `docker build`) by default, or push
    # Push requires using container driver. See https://github.com/docker/buildx#working-with-builder-instances
    output='output = ["type=docker"]'
    if [[ -n "${DOCKERX_PUSH:-}" ]]; then
      output='output = ["type=registry"]'
    fi

    # Wild card check to assign the vm name and version
    # The name of the VM image would be app_sidecar_IMAGE_VERSION
    # Split the $image using "_"
    VM_IMAGE_NAME=""
    VM_IMAGE_VERSION=""
    if [[ "$image" == *"app_sidecar"* ]]; then
      readarray -d _ -t split < <(printf '%s'"$image")
      VM_IMAGE_NAME="${split[-2]}"
      VM_IMAGE_VERSION="${split[-1]}"
    fi

    # Create a list of tags by iterating over HUBs
    tags=""
    for hub in ${HUBS};
    do
      tags=${tags}"\"${hub}/${image}:${tag}\", "
    done
    tags="${tags%, *}" # remove training ', '

    cat <<EOF >> "${config}"
target "$image-$variant" {
    context = "${out}/${file}"
    dockerfile = "Dockerfile.$image"
    tags = [${tags}]
    platforms = [$(to_platform_list "${image}" "${DOCKER_ARCHITECTURES}")]
    args = {
      BASE_VERSION = "${BASE_VERSION}"
      BASE_DISTRIBUTION = "${variant}"
      proxy_version = "istio-proxy:${PROXY_REPO_SHA}"
      istio_version = "${VERSION}"
      VM_IMAGE_NAME = "${VM_IMAGE_NAME}"
      VM_IMAGE_VERSION = "${VM_IMAGE_VERSION}"
    }
    ${output}
}
EOF
    # For the default variant, create an alias so we can do things like `build pilot` instead of `build pilot-default`
    if [[ "${variant}" == "default" ]]; then
    cat <<EOF >> "${config}"
target "$image" {
    inherits = ["$image-$variant"]
}
EOF
    fi
  done
done
