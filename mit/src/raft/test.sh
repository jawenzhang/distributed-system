#!/bin/bash

while [ $? -eq 0 ]; do
    go test -run TestUnreliableChurn
    #go test
done
