#!/usr/bin/env bash

set -o errexit -o nounset -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"

if [ "$#" -ne 1 ]; then
    echo "Usage: hack/release.sh release-version"
    exit 1
fi

release_version=${1#v}

# Verify we are working on a clean directory
require_clean_work_tree () {
    git rev-parse --verify HEAD >/dev/null || exit 1
    git update-index -q --ignore-submodules --refresh
    err=0

    if ! git diff-files --quiet --ignore-submodules
    then
        echo >&2 "Cannot $1: You have unstaged changes."
        err=1
    fi

    if ! git diff-index --cached --quiet --ignore-submodules HEAD --
    then
        if [ $err = 0 ]
        then
            echo >&2 "Cannot $1: Your index contains uncommitted changes."
        else
            echo >&2 "Additionally, your index contains uncommitted changes."
        fi
        err=1
    fi

    if [ $err = 1 ]
    then
        # if there is a 2nd argument print it
        test -n "${2+1}" && echo >&2 "$2"
        exit 1
    fi
}

require_clean_work_tree "release"

# #Verify that you are in a release branch
# if branch=$(git symbolic-ref --short -q HEAD) && [[ "$branch" == release-* ]]
# then
#     echo "Releasing ${release_version}"
# else
#     echo >&2 "Release is not possible because you are not on a 'release-*' branch ($branch)"
#     exit 1
# fi

make kustomize
KUSTOMIZE="${REPO_ROOT}/bin/kustomize"

mkdir -p releases/${release_version}
cp -rv releases/kustomization.yaml.tpl releases/${release_version}/kustomization.yaml
release_manifest="releases/${release_version}/manager.yaml"

# Perform automated substitutions of the version string in the source code
sed -i -e "/Version *= *.*/Is/\".*\"/\"${release_version}\"/" \
    internal/versions/versions.go

CONFIG_TMP_DIR=$(mktemp -d)
cp -r config/* "${CONFIG_TMP_DIR}"
(
    cd "${CONFIG_TMP_DIR}/manager"
    "${KUSTOMIZE}" edit set image controller="ghcr.io/amoniacou/mailgun-operator:${release_version}"
)

"${KUSTOMIZE}" build "${CONFIG_TMP_DIR}/default" > "${release_manifest}"
rm -fr "${CONFIG_TMP_DIR}"

# Create a new branch for the release that originates the PR
git checkout -b "release/v${release_version}"
git add \
    internal/versions/versions.go \
    "releases/${release_version}/*"
git commit -sm "Version tag to ${release_version}"
git push origin -u "release/v${release_version}"

cat <<EOF
Generated release manifest ${release_manifest}
Created and pushed branch release/v${release_version}
EOF
