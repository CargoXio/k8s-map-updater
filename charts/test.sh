#!/usr/bin/env bash
set -e

FIXTURES=$(mktemp -d)
mkdir -p "${FIXTURES}"

reset="$(printf '\033[0m')"
green="$(printf '\033[38;5;46m')"
yellow="$(printf '\033[38;5;178m')"
orange="$(printf '\033[38;5;208m')"
orange_emphasis="$(printf '\033[38;5;220m')"
gray="$(printf '\033[38;5;245m')"


find_charts() {
    find . -mindepth 1 -maxdepth 1 -type d | sed -e 's|^./||g' | sort
}

find_testcases() {
    find . -mindepth 1 -maxdepth 1 -type f -name "$1"'-test*.yml' | sed -e 's|^./||g' | sort -n
}

for chart in $(find_charts); do
    printf "${reset}${orange}==========${reset} ${orange}%s${reset} ${orange}==========${reset}\n" "${chart}"
    for testcase in $(find_testcases "${chart}"); do
        printf "${gray}----------${reset} ${reset}${yellow}%s${reset} ${gray}----------${reset}\n" "${testcase}"
        helm template -f "${testcase}" --dry-run "${chart}" > "${FIXTURES}/${chart}-${testcase}.yml"
        docker run -it -v "${FIXTURES}/:/fixtures/" garethr/kubeval  --strict --schema-location https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/ "fixtures/${chart}-${testcase}.yml"
    done
done

rm -rf "${FIXTURES}"
