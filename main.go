package main

import (
	"flag"
	"fmt"
	"os"
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
	deleteConcurrency      = 1000
	activeHTTPCallCounter  = 0
	maxConcurrentHTTPCalls = 10
	timeFrameSampleCount   = 1000
	s3ProgressObject       = s3ProgressStruct{}
	failedKeysData         string
	completedKeyList       bool
	completedExecution     bool
)

type s3ProgressStruct struct {
	TotalKeys     int
	KeysDeleted   int
	FailedKeys    int
	TimeFrame     []int64
	TotalFileSize int64
}

func main() {
	programStartTime := time.Now()
	parseCommandLineFlags()
	doubleCheckBucketName()
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
	for counter < len(bucketKeys) && len(bucketKeys) > 0 {
		waitGroup.Add(1)
		go func(counter int) {
			for activeHTTPCallCounter >= maxConcurrentHTTPCalls {
				time.Sleep(time.Millisecond * 500)
			}
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
	fmt.Printf("ReEnter the Bucket Name to be emptied : ")
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
		panic(err)
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
			//99 percent it will never hit this block but leaving it as suggested by aws sdk docs
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
		fmt.Print("\033[H\033[2J")
		padSpace := "   "
		string1 := fmt.Sprintf(padSpace+"Total Keys: %v\n", s3ProgressObject.TotalKeys)
		string2 := fmt.Sprintf(padSpace+"Remaining Keys: %v\n", remainingKeys)
		string3 := fmt.Sprintf(padSpace+"Total KeysDeleted: %v\n", s3ProgressObject.KeysDeleted)
		string4 := fmt.Sprintf(padSpace+"Total FailedKeys: %v\n", s3ProgressObject.FailedKeys)
		string5 := fmt.Sprintf(padSpace+"Active Http Calls: %v\n", activeHTTPCallCounter)
		string6 := fmt.Sprintf(padSpace+"Bucket Size(bytes): %v\n", s3ProgressObject.TotalFileSize)
		string7 := fmt.Sprintf(padSpace+"Expected Duration: %v\n", anticipatedDuration)
		concatString := string1 + string2 + string3 + string4 + string5 + string6 + string7
		fmt.Printf("Execution Stats(%s):\n"+concatString+"%s", bucketName, getProgressString(progress))
		if progressRecieved != 100 {
			time.Sleep(time.Millisecond * 300)
		} else {
			break
		}
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
