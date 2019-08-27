package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tealeg/xlsx"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

func main() {
	opType := os.Args[1]

	repoDir := os.Args[2]
	srcBranch := os.Args[3]
	dstBranch := os.Args[4]

	var mergeExcel  *xlsx.File = nil
	var excelAbsPath string = ""
	var err error = nil

	if opType!="getCommitLogs" {
		//git commit logs是不需要打开merge.xlsx
		excelFile := os.Args[5]

		excelAbsPath = path.Join(repoDir, excelFile)

		_,err = os.Stat(excelAbsPath)
		if err != nil {
			mergeExcel = xlsx.NewFile()
		} else {
			mergeExcel, err = xlsx.OpenFile(excelAbsPath)
			CheckIfError(err)
		}
	}

	if opType == "getCommitLogs" {

		r, err := git.PlainOpen(repoDir)
		CheckIfError(err)

		commitLogs, err := getCommitLogs(r, srcBranch, dstBranch)
		CheckIfError(err)

		saveCommitLogsToTmpExcel(commitLogs)
	} else if opType == "getMergeList" {
		r, err := git.PlainOpen(repoDir)
		CheckIfError(err)

		commitLogsExcel,err := xlsx.OpenFile(getTmpCommitLogsExcelPath())
		CheckIfError(err)

		//从src分支git log获取的所有最新的commit items
		latestCommitItems, _ := loadCommitMsgFromExcel(commitLogsExcel)

		//从merge.xlsx中获取到已经被merge到dst分支的commit items
		_, mergedItems := loadCommitMsgFromExcel(mergeExcel)

		//把已经Merge过的item填上merged time
		fillMergedItems(latestCommitItems, mergedItems)

		needMergeItems := getNeedMergeJiraIds(mergeExcel)

		writeMergeListToStdout(r, latestCommitItems, needMergeItems)

		//回填excel,把最新的commit logs填到sheet1
		writeBackToExcel(mergeExcel, excelAbsPath, latestCommitItems)
	} else if opType == "finishMerge" {

		mergeList := os.Args[6]
		mergeHashs := strings.Split(mergeList, ",")

		commitItemsFromExcel, _ := loadCommitMsgFromExcel(mergeExcel)

		//在sheet1中，把完成合并的条目天上merge time
		fillFinishMergeList(mergeHashs, commitItemsFromExcel)

		writeBackToExcel(mergeExcel, excelAbsPath, commitItemsFromExcel)
	} else {
		fmt.Fprintln(os.Stdout, "错误的操作类型")
	}

}

func fillMergedItems(latestCommitItems []*MergeCommitItem, mergedItems map[string]*MergeCommitItem)  {
	for _,commitItem := range latestCommitItems {
		if mergedItem,ok := mergedItems[commitItem.commitHash]; ok {
			commitItem.mergedTime = mergedItem.mergedTime
		}
	}
}

//获取PM整理的待合单子列表
func getNeedMergeJiraIds(mergeExcelFile *xlsx.File) []string {
	//所有的commit条目
	needMergeItems := make([]string, 0)

	if len(mergeExcelFile.Sheets) > 1 {
		needMergeSheet := mergeExcelFile.Sheets[1]
		for _, row := range needMergeSheet.Rows {
			if len(row.Cells) < 1 {
				continue
			}
			needMergeItems = append(needMergeItems, row.Cells[0].Value)
		}
	}

	return needMergeItems
}

//把获取到的git logs保存在一个临时文件里
func saveCommitLogsToTmpExcel(commitLogs []*MergeCommitItem)  {
	tmpLogsExcel := xlsx.NewFile()

	writeBackToExcel(tmpLogsExcel, getTmpCommitLogsExcelPath(), commitLogs)
}

func getTmpCommitLogsExcelPath() string {
	tmpExcelPath,_ := filepath.Abs("TempCommitLogs.xlsx")
	return tmpExcelPath
}

//获取src分支的提交记录
func getCommitLogs(repository *git.Repository, srcBranch, dstBranch string) (allCommitItems []*MergeCommitItem, err error) {
	//所有的commit条目
	allCommitItems = make([]*MergeCommitItem, 0)

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
	err = cIter.ForEach(func(commitLogItem *object.Commit) error {
		//logs = append(logs, c)

		commitItem := &MergeCommitItem{}
		commitItem.commitHash = commitLogItem.Hash.String()[:7] //commit hash
		commitItem.mergedTime = ""
		commitItem.commitMsg = commitLogItem.Message
		commitItem.author = commitLogItem.Committer.Name
		commitItem.mail = commitLogItem.Committer.Email
		commitItem.commitTime = commitLogItem.Committer.When.Format("2006-01-02 15:04:05")

		allCommitItems = append(allCommitItems, commitItem)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return allCommitItems, err
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

//把需要merge的结果写到stdout里，这样执行merge的脚本就能获取到了
func writeMergeListToStdout(repository *git.Repository, allCommitItems []*MergeCommitItem, needMergeItems []string) error {
	var mergeList string = ""

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

	if len(mergeExcelFile.Sheets) < 2 {
		mergeExcelFile.AddSheet("待merge单号")
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
