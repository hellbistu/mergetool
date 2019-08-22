package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/tealeg/xlsx"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

func main() {
	opType := os.Args[1]

	repoDir := os.Args[2]
	srcBranch := os.Args[3]
	dstBranch := os.Args[4]
	excelFile := os.Args[5]

	excelAbsPath := path.Join(repoDir, excelFile)

	mergeExcel, err := xlsx.OpenFile(excelAbsPath)
	CheckIfError(err)

	if opType == "getMergeList" {
		r, err := git.PlainOpen(repoDir)
		CheckIfError(err)

		commitHistorys, err := getCommitLogs(r, srcBranch, dstBranch)
		CheckIfError(err)

		allCommitItems, neetMergeItems := parseMergeContent(commitHistorys, mergeExcel)

		genMergeList(r, allCommitItems, neetMergeItems)

		//回填excel,把最新的commit logs填到sheet1
		writeBackToExcel(mergeExcel, excelAbsPath, allCommitItems)
	} else if opType == "finishMerge" {

		mergeList := os.Args[6]
		mergeHashs := strings.Split(mergeList, ",")

		commitItemsFromExcel, _ := loadCommitMsgFromExcel(mergeExcel)

		//在sheet1中，把完成合并的条目天上merge time
		fillFinishMergeList(mergeHashs, commitItemsFromExcel)

		writeBackToExcel(mergeExcel, excelAbsPath, commitItemsFromExcel)
	} else if opType == "commitHashToMsg" {
		commitItemsFromExcel, _ := loadCommitMsgFromExcel(mergeExcel)
		commitHash := os.Args[6]
		commitMsg := hashToCommitMsg(commitHash, commitItemsFromExcel)

		//写回os.stdout里，merge的shell脚本会读取到
		fmt.Fprintln(os.Stdout, commitMsg)
	} else {
		fmt.Fprintln(os.Stdout, "错误的操作类型")
	}

}

//获取src分支的提交记录
func getCommitLogs(repository *git.Repository, srcBranch, dstBranch string) ([]*object.Commit, error) {
	logs := make([]*object.Commit, 0)

	w, err := repository.Worktree()
	if err != nil {
		return nil, err
	}

	//切到src分支之前先stash
	stashCmd := exec.Command("git", "stash")
	err = stashCmd.Run()
	if err != nil {
		return nil, err
	}

	//先切到src分支,抓到所有的提交日志
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(srcBranch),
	})

	if err != nil {
		return nil, err
	}

	//指针移动到HEAD
	ref, err := repository.Head()
	if err != nil {
		return nil, err
	}

	//查看提交log
	cIter, err := repository.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	//遍历
	err = cIter.ForEach(func(c *object.Commit) error {
		logs = append(logs, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	//抓取完src的提交日志，切换回dst分支，开始cherry-pick
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(dstBranch),
		Force:  true,
	})
	if err != nil {
		return nil, err
	}

	stashApplyCmd := exec.Command("git", "stash", "pop", "-q")
	err = stashApplyCmd.Run()
	if err != nil {
		err = nil
	}

	return logs, err
}

//解析merge内容
//allCommitItems 所有的提交记录
//needMergeItems 由PM整理的本次需要合并的单子
func parseMergeContent(commitLogs []*object.Commit, mergeExcelFile *xlsx.File) (allCommitItems []*MergeCommitItem, needMergeItems []string) {

	//所有的commit条目
	allCommitItems = make([]*MergeCommitItem, 0)
	needMergeItems = make([]string, 0)

	_, commitHashsFromExcel := loadCommitMsgFromExcel(mergeExcelFile)

	for _, commitLogItem := range commitLogs {
		commitHash := commitLogItem.Hash.String()[:7]
		if commitItem, ok := commitHashsFromExcel[commitHash]; ok {
			//之前已经在excel merge存档里的，跳过
			allCommitItems = append(allCommitItems, commitItem)
			continue
		}

		commitItem := &MergeCommitItem{}
		commitItem.commitHash = commitLogItem.Hash.String()[:7] //commit hash
		commitItem.mergedTime = ""
		commitItem.commitMsg = commitLogItem.Message
		commitItem.author = commitLogItem.Committer.Name
		commitItem.mail = commitLogItem.Committer.Email
		commitItem.commitTime = commitLogItem.Committer.When.Format("2006-01-02 15:04:05")

		allCommitItems = append(allCommitItems, commitItem)
	}

	if len(mergeExcelFile.Sheets) > 1 {
		needMergeSheet := mergeExcelFile.Sheets[1]
		for _, row := range needMergeSheet.Rows {
			if len(row.Cells) < 1 {
				continue
			}
			needMergeItems = append(needMergeItems, row.Cells[0].Value)
		}
	}

	//merge的时候，需要按照提交顺序从 老的 到 新的 合并

	return allCommitItems, needMergeItems
}

//从excel加载之前的commit log
func loadCommitMsgFromExcel(mergeExcelFile *xlsx.File) ([]*MergeCommitItem, map[string]*MergeCommitItem) {
	commitItems := make([]*MergeCommitItem, 0)
	commitHashs := make(map[string]*MergeCommitItem)

	if len(mergeExcelFile.Sheets) < 1 {
		return commitItems, commitHashs
	}

	mergeProfileSheet := mergeExcelFile.Sheets[0]

	for _, row := range mergeProfileSheet.Rows {
		commitItem := &MergeCommitItem{}
		commitItem.commitHash = row.Cells[0].Value
		commitItem.mergedTime = row.Cells[1].Value
		commitItem.commitMsg = row.Cells[2].Value
		commitItem.author = row.Cells[3].Value
		commitItem.mail = row.Cells[4].Value
		commitItem.commitTime = row.Cells[5].Value

		commitItems = append(commitItems, commitItem)
		commitHashs[commitItem.commitHash] = commitItem
	}

	return commitItems, commitHashs
}

//开始合并
func genMergeList(repository *git.Repository, allCommitItems []*MergeCommitItem, needMergeItems []string) error {
	var mergeList string = ""
	//fmt.Fprintf(os.Stdout, "开始merge\n")
	firstMergeHash := true
	for i := len(allCommitItems) - 1; i >= 0; i-- {
		commitItem := allCommitItems[i]
		if strings.TrimSpace(commitItem.mergedTime) != "" {
			//已经被合并了
			continue
		}

		if !matchMergeRule(commitItem, needMergeItems) {
			continue
		}

		if !firstMergeHash {
			mergeList = fmt.Sprintf("%s,", mergeList)
		}
		firstMergeHash = false
		mergeList = fmt.Sprintf("%s%s", mergeList, commitItem.commitHash)
	}

	//写回os.stdout里，merge的shell脚本会读取到
	fmt.Fprintln(os.Stdout, mergeList)

	return nil
}

//判断是否符合merge要求
func matchMergeRule(commitItem *MergeCommitItem, needMergeItems []string) bool {
	for _, needMergeMsg := range needMergeItems {
		if strings.Contains(commitItem.commitMsg, needMergeMsg) {
			return true
		}
	}
	return false
}

func writeBackToExcel(mergeExcelFile *xlsx.File, saveFilePath string, allCommitItems []*MergeCommitItem) {
	defer mergeExcelFile.Save(saveFilePath)

	if len(allCommitItems) > 0 {
		if len(mergeExcelFile.Sheets) < 1 {
			mergeExcelFile.AddSheet("merge存档")
		}

		commitItemSheet := mergeExcelFile.Sheets[0]
		commitItemSheet.Rows = make([]*xlsx.Row, 0)
		for _, commitItem := range allCommitItems {
			row := commitItemSheet.AddRow()
			for len(row.Cells) < 6 {
				row.AddCell()
			}

			row.Cells[0].Value = commitItem.commitHash
			row.Cells[1].Value = commitItem.mergedTime
			row.Cells[2].Value = commitItem.commitMsg
			row.Cells[3].Value = commitItem.author
			row.Cells[4].Value = commitItem.mail
			row.Cells[5].Value = commitItem.commitTime
		}
	}
}

func fillFinishMergeList(finishHash []string, commItems []*MergeCommitItem) {
	if len(commItems) < 0 {
		return
	}
	for _, commitItem := range commItems {

		//之前已经合并过的
		if strings.TrimSpace(commitItem.mergedTime) != "" {
			continue
		}

		if isMerged(finishHash, commitItem.commitHash) {
			commitItem.mergedTime = time.Now().Format("2006-01-02 15:04:05")
		}
	}
}

func hashToCommitMsg(commitHash string, commItems []*MergeCommitItem) string {
	for _, commitItem := range commItems {
		if commitHash == commitItem.commitHash {
			return commitItem.commitMsg
		}
	}

	return ""
}

func isMerged(finishHashs []string, hash string) bool {
	for _, finishHash := range finishHashs {
		if finishHash == hash {
			return true
		}
	}
	return false
}

type MergeCommitItem struct {
	author     string
	mail       string
	commitTime string
	commitHash string
	mergedTime string
	commitMsg  string
}

func CheckIfError(err error) {
	if err == nil {
		return
	}

	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}
