#!/bin/sh

#####################################config start#################################
#源分支，从这个分支merge内容
srcBranch="test-trunk"

#目标分支,向这个分支merge内容
dstBranch="test_branch"

#工程的git目录
repoDir="D:\comicserver"

#excel名字
excelFile="merge.xlsx"
#####################################config  end#################################

#当前工作目录，脚本，merge tool所在目录
workDir=`pwd`

#切到工程git目录
cd $repoDir

MergeTool="$workDir/merge-tool-win64"
echo "merge tool path is:    $MergeTool"

#先pull到最新
echo "执行git pull更至最新"
git pull > /dev/null

#先stash
git stash > /dev/null

#先切换到src分支获取最新的commit logs
git checkout $srcBranch > /dev/null
git pull > /dev/null

cd $workDir
$MergeTool getCommitLogs ${repoDir} ${srcBranch} ${dstBranch}
rtnCode=$?
if [ $rtnCode -ne 0 ] ; then
    echo "get latest commit logs failed!"
    exit 1
fi

#切回到dst分支
cd $repoDir
git checkout $dstBranch > /dev/null
git stash pop > /dev/null

cd $workDir
mergeList=`$MergeTool getMergeList ${repoDir} ${srcBranch} ${dstBranch} ${excelFile}`
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

cd $repoDir
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
			cd $workDir
            exit 1
        fi
    
        #解决完冲突,需要cherry-pick --continue
        git cherry-pick --continue
        cherryPickContinueCode=$?
        if [ $cherryPickContinueCode -ne 0 ] ; then
            git cherry-pick --abort
            echo "解决冲突失败，请把当前工作目录重置干净(git reset --hard)再试"
			cd $workDir
            exit 1
        fi
    fi
done

rm -rf *.orig

#把merge记录回填到excel中
cd $workDir
$MergeTool finishMerge ${repoDir} ${srcBranch} ${dstBranch} ${excelFile} ${mergeList}

cd $repoDir
git add $excelFile
git commit -m "[auto commit by mergetool]merge msg: $mergeList"

echo "#########################################################################################################"
echo "merge 完成，本次merge的条目为: $mergeList"
echo "#########################################################################################################"

cd $workDir
rm TempCommitLogs.xlsx > /dev/null
