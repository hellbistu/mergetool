#!/bin/sh

#####################################config start#################################
#源分支，从这个分支merge内容
srcBranch="test-trunk"

#目标分支,向这个分支merge内容
dstBranch="test_branch"

#excel名字
excelFile="merge.xlsx"
#####################################config  end#################################

repoDir=`pwd`

#先pull到最新
echo "执行git pull更至最新"
git pull

mergeList=`./git-tool getMergeList ${repoDir} ${srcBranch} ${dstBranch} ${excelFile}`
rtnCode=$?

if [ $rtnCode -ne 0 ] ; then
    echo "get merge list failed!"
    exit 1
fi

if [ -z $mergeList ]; then
    echo "本次需要merge0条记录"
    exit 0
fi

echo "start merge ${mergeList}"

hashArr=(${mergeList//,/ })  
for hash in ${hashArr[@]}  
do  
    echo "开始merge    ->      $hash"
    git cherry-pick $hash>/dev/null
    cherryPickCode=$?
    if [ $cherryPickCode -ne 0 ] ; then
        git mergetool
        resolveConflictCode=$?
        if [ $resolveConflictCode -ne 0 ] ; then
            git cherry-pick --abort
            echo "解决冲突失败，请把当前工作目录重置干净(git reset --hard)再试"
            exit 1
        fi
    
        #解决完冲突,需要cherry-pick --continue
        git cherry-pick --continue
        cherryPickContinueCode=$?
        if [ $cherryPickContinueCode -ne 0 ] ; then
            git cherry-pick --abort
            echo "解决冲突失败，请把当前工作目录重置干净(git reset --hard)再试"
            exit 1
        fi
    fi
done

rm -rf *.orig

#把merge记录回填到excel中
./git-tool finishMerge ${repoDir} ${srcBranch} ${dstBranch} ${excelFile} ${mergeList}

git add $excelFile
git commit -m "[auto commit by mergetool]merge msg: $mergeList"

echo "#########################################################################################################"
echo "merge 完成，本次merge的条目为: $mergeList"
echo "#########################################################################################################"

