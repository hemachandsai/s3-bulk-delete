package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var (
	s3Session              *s3.S3
	bucketName             string
	awsRegion              string
	bucketKeys             = []string{}
	waitGroup              = &sync.WaitGroup{}
	queryConcurreny        = int64(1000)
	deleteConcurrency      = 500
	activeHTTPCallCounter  = 0
	maxConcurrentHTTPCalls = 7
	timeFrameSampleCount   = 1000
	s3ProgressObject       = s3ProgressStruct{}
	failedKeysData         string
	completedKeyList       bool
	completedExecution     bool
	sleepingCount          = 0
	clearANSISequence      = "\033[H\033[2J\033[3J"
	isWindows              bool
)

type s3ProgressStruct struct {
	TotalKeys     int
	KeysDeleted   int
	FailedKeys    int
	TimeFrame     []int64
	TotalFileSize int64
}

func main() {
	if runtime.GOOS == "windows" {
		isWindows = true
	}
	parseCommandLineFlags()
	doubleCheckBucketName()

	programStartTime := time.Now()
	go logToTerminal(0)

	newSession := session.Must(session.NewSession())
	s3Session = s3.New(newSession, aws.NewConfig().WithRegion(awsRegion))
	traversedAllFiles := false
	lastKey := ""
	for !traversedAllFiles {
		result, lastKeyrec := getS3ObjectsList(lastKey)
		if !result {
			return
		}
		lastKey = lastKeyrec
		if result && lastKeyrec == "" {
			traversedAllFiles = true
		}
	}
	completedKeyList = true
	counter := 0
	bucketKeys = append(bucketKeys)
outerLoop:
	for range time.Tick(time.Second) {
		for i := 0; i < maxConcurrentHTTPCalls; i++ {
			if counter < len(bucketKeys) && len(bucketKeys) > 0 {
				waitGroup.Add(1)
				go func(counter int) {
					activeHTTPCallCounter++
					if counter+deleteConcurrency < len(bucketKeys) {
						deleteS3Objects(bucketKeys[counter : counter+deleteConcurrency])
					} else {
						deleteS3Objects(bucketKeys[counter:len(bucketKeys)])
					}
					defer func() {
						activeHTTPCallCounter--
						waitGroup.Done()
					}()
				}(counter)
				counter += deleteConcurrency
			} else {
				break outerLoop
			}
		}
	}
	waitGroup.Wait()
	completedExecution = true
	logToTerminal(100)
	if len(failedKeysData) > 0 {
		fmt.Println("Failed Keys Data: ", failedKeysData)
	}
	fmt.Println("Completed Deletion of files in the Bucket. Total time taken: ", time.Since(programStartTime))
}

func parseCommandLineFlags() {
	bucketNamePtr := flag.String("bucket", "", "Bucket name to be deleted. (Required)")
	region := flag.String("aws-region", "", "AWS Region in which bucket exists. (Required)")
	flag.Parse()
	if *bucketNamePtr == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *region == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	bucketName = *bucketNamePtr
	awsRegion = *region
}

func doubleCheckBucketName() {
	var enteredBucketName string
	fmt.Printf("Re-Enter the Bucket Name to be emptied : ")
	_, err := fmt.Scanln(&enteredBucketName)
	if err != nil {
		logError(err.Error())
		os.Exit(1)
	}
	if enteredBucketName != bucketName {
		logError("Bucket name doesnt match the previous entered value. Please re-run the script")
		os.Exit(1)
	}
}

func deleteS3Objects(s3Keys []string) {
	startTime := time.Now().Unix()
	deleteObjectsStruct := s3.Delete{
		Objects: []*s3.ObjectIdentifier{},
	}
	for _, keyname := range s3Keys {
		objectIdentifier := s3.ObjectIdentifier{
			Key: aws.String(keyname),
		}
		deleteObjectsStruct.Objects = append(deleteObjectsStruct.Objects, &objectIdentifier)
	}
	deleteObjectsInput := s3.DeleteObjectsInput{
		Bucket: aws.String(bucketName),
		Delete: &deleteObjectsStruct,
	}
	deleteObjectsOutput, err := s3Session.DeleteObjects(&deleteObjectsInput)
	if err != nil {
		if awsErr, ok := err.(awserr.RequestFailure); ok {
			if awsErr.StatusCode() == 503 {
				logError("AWS AmazonS3Exception SlowDown Error. Exiting now. Please retry after 5 seconds...")
				os.Exit(1)
			} else {
				panic(err)
			}
		} else {
			panic(err)
		}
		return
	}
	s3ProgressObject.KeysDeleted += len(deleteObjectsOutput.Deleted)
	s3ProgressObject.FailedKeys += len(deleteObjectsOutput.Errors)
	for _, value := range deleteObjectsOutput.Errors {
		failedKeysData += *value.Key + "\n"
	}
	timeTaken := time.Now().Unix() - startTime
	if len(s3ProgressObject.TimeFrame) >= timeFrameSampleCount {
		s3ProgressObject.TimeFrame = s3ProgressObject.TimeFrame[1:len(s3ProgressObject.TimeFrame)]
		s3ProgressObject.TimeFrame = append(s3ProgressObject.TimeFrame, timeTaken)
	} else {
		s3ProgressObject.TimeFrame = append(s3ProgressObject.TimeFrame, timeTaken)
	}
}

func getS3ObjectsList(lastKey string) (bool, string) {
	s3ListInput := s3.ListObjectsInput{
		Bucket:  aws.String(bucketName),
		MaxKeys: aws.Int64(queryConcurreny),
	}
	if lastKey != "" {
		s3ListInput.Marker = aws.String(lastKey)
	}
	s3ListOutput, err := s3Session.ListObjects(&s3ListInput)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == credentials.ErrNoValidProvidersFoundInChain.Code() {
				logError("AWS authentication failed. Please configure aws-cli in the system or load the access_key_id, aws_secret_access_key to AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY environment variables.\nPlease refer to https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials for more info")
			} else if awsErr.Code() == "BucketRegionError" {
				logError("Bucket not available in the specified region...")
			} else if awsErr.Code() == s3.ErrCodeNoSuchBucket {
				logError("No Resource found with the given BucketName. Please double check input...")
			} else {
				logError("Error: " + awsErr.Code() + " " + awsErr.Message())
			}
			os.Exit(1)
		} else {
			panic(err.Error())
		}
	}
	s3ProgressObject.TotalKeys += len(s3ListOutput.Contents)
	var lastKeyName string
	for index, value := range s3ListOutput.Contents {
		s3ProgressObject.TotalFileSize += *value.Size
		bucketKeys = append(bucketKeys, *value.Key)
		if index == len(s3ListOutput.Contents)-1 {
			lastKeyName = *value.Key
		}
	}
	if *s3ListOutput.IsTruncated {
		return true, lastKeyName
	}
	return true, ""
}

func logToTerminal(progressRecieved int) {
	for !completedExecution || progressRecieved == 100 {
		progress := float64(0)
		anticipatedDuration := time.Duration(0) * time.Second
		remainingKeys := 0
		if completedKeyList {
			progress += float64(20)
			remainingKeys = s3ProgressObject.TotalKeys - s3ProgressObject.KeysDeleted - s3ProgressObject.FailedKeys
			if remainingKeys == 0 {
				progress = float64(100)
			} else {
				progress += (float64(s3ProgressObject.TotalKeys-remainingKeys)/float64(s3ProgressObject.TotalKeys))*float64(100) - 20
			}
			anticipatedDuration = time.Duration(int(getAverageTimeFrame()*float64(remainingKeys)/float64(deleteConcurrency))/maxConcurrentHTTPCalls) * time.Second
		}
		if progressRecieved == 100 {
			progress = float64(progressRecieved)
		}
		clearTerminal()
		string1 := fmt.Sprintf("\tTotal Keys: %v\n", s3ProgressObject.TotalKeys)
		string2 := fmt.Sprintf("\tRemaining Keys: %v\n", remainingKeys)
		string3 := fmt.Sprintf("\tTotal KeysDeleted: %v\n", s3ProgressObject.KeysDeleted)
		string4 := fmt.Sprintf("\tTotal FailedKeys: %v\n", s3ProgressObject.FailedKeys)
		string5 := fmt.Sprintf("\tActive Http Calls: %v\n", activeHTTPCallCounter)
		string6 := fmt.Sprintf("\tBucket Size(Mb): %.2f\n", float64(s3ProgressObject.TotalFileSize)/float64(1024))
		string7 := fmt.Sprintf("\tExpected Duration: %v\n", anticipatedDuration)
		concatString := string1 + string2 + string3 + string4 + string5 + string6 + string7
		fmt.Printf("Execution Stats(%s):\n"+concatString+"%s", bucketName, getProgressString(progress))
		if progressRecieved != 100 {
			time.Sleep(time.Millisecond * 300)
		} else {
			break
		}
	}
}

func clearTerminal() {
	if isWindows {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Print(clearANSISequence)
	}
}

func getAverageTimeFrame() float64 {
	if len(s3ProgressObject.TimeFrame) == 0 {
		return 0
	}
	totalDuration := int64(0)
	for _, value := range s3ProgressObject.TimeFrame {
		totalDuration += value
	}
	return float64(totalDuration) / float64(len(s3ProgressObject.TimeFrame))
}

func getProgressString(currentProgress float64) string {
	var text string
	if currentProgress == 100 {
		text = "Completed ["
	} else {
		text = "Ongoing ["
	}
	for i := 0; i <= int(currentProgress); i++ {
		text += "="
	}
	text += ">"
	for i := 0; i < 100-int(currentProgress); i++ {
		text += " "
	}
	text += "] " + fmt.Sprintf("%.2f", currentProgress) + "%\n"
	return text
}

func logError(msg string) {
	redColor := "\033[31m"
	reset := "\033[0m"
	fmt.Println(redColor + msg + reset)
}
