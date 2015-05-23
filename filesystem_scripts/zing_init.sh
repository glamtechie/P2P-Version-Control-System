#!/bin/bash

#Initialize local repository
git init &> /dev/null;

#Add .zing dir that will contain all metadata to gitignore
echo ".zing" > .gitignore
if [ -d ".zing" ]; then
    rm -rf .zing
    echo "Reinitialized existing zing repository in $PWD/.zing"
else
    echo "Initialized zing repository in $PWD/.zing"
fi

pushd . &> /dev/null;

mkdir .zing && cd .zing;

#This ID needs to be a unique per node identifier
echo "ID:#" > config

#global dir that hold the pushes
mkdir global && cd global;
git init &> /dev/null;

popd &> /dev/null;


