
name: Build the TFO Mutator Container Image

on:
  pull_request:
    branches:
    - master
  push:
    tags:
    - '*'
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
    - run: pip install docker
    - name: Checkout
      uses: actions/checkout@v3
      with:
        fetch-depth: 0

    - name: Log in to registry
      # This is where you will update the PAT to GITHUB_TOKEN
      run: echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u $ --password-stdin

    - name: release
      run: |
        version=$(git describe --tags --dirty)
        img="ghcr.io/galleybytes/terraform-operator-mutator:${version:-0.0.0}"
        docker build . -t "$img"
        docker push "$img"
