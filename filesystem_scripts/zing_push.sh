#!/bin/bash


#1 == branchname, 2 == patch name
pushd . &> /dev/null;

cd .zing/global;

git checkout -b temp &> /dev/null;

git log &> err
line=$(head -n 1 err)
rm err
if [ "$line" == "fatal: bad default revision 'HEAD'" ]; then
    temp_head_first="--root"
else

    temp_head_first=`git log --pretty=oneline | sed -n '1p' | awk '{print $1}'`
fi

#echo "$temp_head_first"
git pull ../../ $1  &> /dev/null;

git format-patch $temp_head_first --stdout > $2 

popd &> /dev/null;


