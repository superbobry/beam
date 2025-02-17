// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package code_processing

import (
	pb "beam.apache.org/playground/backend/internal/api/v1"
	"beam.apache.org/playground/backend/internal/cache"
	"beam.apache.org/playground/backend/internal/environment"
	"beam.apache.org/playground/backend/internal/errors"
	"beam.apache.org/playground/backend/internal/executors"
	"beam.apache.org/playground/backend/internal/fs_tool"
	"beam.apache.org/playground/backend/internal/logger"
	"beam.apache.org/playground/backend/internal/setup_tools/builder"
	"beam.apache.org/playground/backend/internal/streaming"
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os/exec"
	"time"
)

// Process validates, compiles and runs code by pipelineId.
// During each operation updates status of execution and saves it into cache:
// - In case of processing works more that timeout duration saves playground.Status_STATUS_RUN_TIMEOUT as cache.Status into cache.
// - In case of code processing has been canceled saves playground.Status_STATUS_CANCELED as cache.Status into cache.
// - In case of validation step is failed saves playground.Status_STATUS_VALIDATION_ERROR as cache.Status into cache.
// - In case of compile step is failed saves playground.Status_STATUS_COMPILE_ERROR as cache.Status and compile logs as cache.CompileOutput into cache.
// - In case of compile step is completed with no errors saves compile output as cache.CompileOutput into cache.
// - In case of run step is failed saves playground.Status_STATUS_RUN_ERROR as cache.Status and run logs as cache.RunError into cache.
// - In case of run step is completed with no errors saves playground.Status_STATUS_FINISHED as cache.Status and run output as cache.RunOutput into cache.
// At the end of this method deletes all created folders.
func Process(ctx context.Context, cacheService cache.Cache, lc *fs_tool.LifeCycle, pipelineId uuid.UUID, appEnv *environment.ApplicationEnvs, sdkEnv *environment.BeamEnvs) {
	ctxWithTimeout, finishCtxFunc := context.WithTimeout(ctx, appEnv.PipelineExecuteTimeout())
	defer func(lc *fs_tool.LifeCycle) {
		finishCtxFunc()
		DeleteFolders(pipelineId, lc)
	}(lc)

	errorChannel := make(chan error, 1)
	successChannel := make(chan bool, 1)
	cancelChannel := make(chan bool, 1)

	go cancelCheck(ctxWithTimeout, pipelineId, cancelChannel, cacheService)

	executorBuilder, err := builder.SetupExecutorBuilder(lc.GetAbsoluteSourceFilePath(), lc.GetAbsoluteBaseFolderPath(), lc.GetAbsoluteExecutableFilePath(), sdkEnv)
	if err != nil {
		processSetupError(err, pipelineId, cacheService, ctxWithTimeout)
		return
	}
	executor := executorBuilder.Build()

	// Validate
	logger.Infof("%s: Validate() ...\n", pipelineId)
	validateFunc := executor.Validate()
	go validateFunc(successChannel, errorChannel)

	if err = processStep(ctxWithTimeout, pipelineId, cacheService, cancelChannel, successChannel, nil, nil, errorChannel, pb.Status_STATUS_VALIDATION_ERROR, pb.Status_STATUS_PREPARING); err != nil {
		return
	}

	// Prepare
	logger.Infof("%s: Prepare() ...\n", pipelineId)
	prepareFunc := executor.Prepare()
	go prepareFunc(successChannel, errorChannel)

	if err = processStep(ctxWithTimeout, pipelineId, cacheService, cancelChannel, successChannel, nil, nil, errorChannel, pb.Status_STATUS_PREPARATION_ERROR, pb.Status_STATUS_COMPILING); err != nil {
		return
	}

	switch sdkEnv.ApacheBeamSdk {
	case pb.Sdk_SDK_JAVA, pb.Sdk_SDK_GO:
		// Compile
		logger.Infof("%s: Compile() ...\n", pipelineId)
		compileCmd := executor.Compile(ctxWithTimeout)
		var compileError bytes.Buffer
		var compileOutput bytes.Buffer
		runCmdWithOutput(compileCmd, &compileOutput, &compileError, successChannel, errorChannel)

		if err = processStep(ctxWithTimeout, pipelineId, cacheService, cancelChannel, successChannel, &compileOutput, &compileError, errorChannel, pb.Status_STATUS_COMPILE_ERROR, pb.Status_STATUS_EXECUTING); err != nil {
			return
		}
	case pb.Sdk_SDK_PYTHON:
		processSuccess(ctx, []byte(""), pipelineId, cacheService, pb.Status_STATUS_EXECUTING)
	}

	// Run
	if sdkEnv.ApacheBeamSdk == pb.Sdk_SDK_JAVA {
		executor = setJavaExecutableFile(lc, pipelineId, cacheService, ctxWithTimeout, executorBuilder, appEnv.WorkingDir())
	}
	logger.Infof("%s: Run() ...\n", pipelineId)
	runCmd := executor.Run(ctxWithTimeout)
	var runError bytes.Buffer
	runOutput := streaming.RunOutputWriter{Ctx: ctxWithTimeout, CacheService: cacheService, PipelineId: pipelineId}
	runCmdWithOutput(runCmd, &runOutput, &runError, successChannel, errorChannel)

	err = processStep(ctxWithTimeout, pipelineId, cacheService, cancelChannel, successChannel, nil, &runError, errorChannel, pb.Status_STATUS_RUN_ERROR, pb.Status_STATUS_FINISHED)
	if err != nil {
		return
	}
}

// setJavaExecutableFile sets executable file name to runner (JAVA class name is known after compilation step)
func setJavaExecutableFile(lc *fs_tool.LifeCycle, id uuid.UUID, service cache.Cache, ctx context.Context, executorBuilder *executors.ExecutorBuilder, dir string) executors.Executor {
	className, err := lc.ExecutableName(id, dir)
	if err != nil {
		processSetupError(err, id, service, ctx)
	}
	return executorBuilder.WithRunner().WithExecutableFileName(className).Build()
}

// processSetupError processes errors during the setting up an executor builder
func processSetupError(err error, pipelineId uuid.UUID, cacheService cache.Cache, ctxWithTimeout context.Context) {
	logger.Errorf("%s: error during setup builder: %s\n", pipelineId, err.Error())
	cacheService.SetValue(ctxWithTimeout, pipelineId, cache.Status, pb.Status_STATUS_ERROR)
}

// GetProcessingOutput gets processing output value from cache by key and subKey.
// In case key doesn't exist in cache - returns an errors.NotFoundError.
// In case subKey doesn't exist in cache for the key - returns an errors.NotFoundError.
// In case value from cache by key and subKey couldn't be converted to string - returns an errors.InternalError.
func GetProcessingOutput(ctx context.Context, cacheService cache.Cache, key uuid.UUID, subKey cache.SubKey, errorTitle string) (string, error) {
	value, err := cacheService.GetValue(ctx, key, subKey)
	if err != nil {
		logger.Errorf("%s: GetStringValueFromCache(): cache.GetValue: error: %s", key, err.Error())
		return "", errors.NotFoundError(errorTitle, fmt.Sprintf("Error during getting cache by key: %s, subKey: %s", key.String(), string(subKey)))
	}
	stringValue, converted := value.(string)
	if !converted {
		logger.Errorf("%s: couldn't convert value to string: %s", key, value)
		return "", errors.InternalError(errorTitle, fmt.Sprintf("Value from cache couldn't be converted to string: %s", value))
	}
	return stringValue, nil
}

// GetProcessingStatus gets processing status from cache by key.
// In case key doesn't exist in cache - returns an errors.NotFoundError.
// In case value from cache by key and subKey couldn't be converted to playground.Status - returns an errors.InternalError.
func GetProcessingStatus(ctx context.Context, cacheService cache.Cache, key uuid.UUID, errorTitle string) (pb.Status, error) {
	value, err := cacheService.GetValue(ctx, key, cache.Status)
	if err != nil {
		logger.Errorf("%s: GetStringValueFromCache(): cache.GetValue: error: %s", key, err.Error())
		return pb.Status_STATUS_UNSPECIFIED, errors.NotFoundError(errorTitle, fmt.Sprintf("Error during getting cache by key: %s, subKey: %s", key.String(), string(cache.Status)))
	}
	statusValue, converted := value.(pb.Status)
	if !converted {
		logger.Errorf("%s: couldn't convert value to correct status enum: %s", key, value)
		return pb.Status_STATUS_UNSPECIFIED, errors.InternalError(errorTitle, fmt.Sprintf("Value from cache couldn't be converted to correct status enum: %s", value))
	}
	return statusValue, nil
}

// GetLastIndex gets last index for run output or logs from cache by key.
// In case key doesn't exist in cache - returns an errors.NotFoundError.
// In case value from cache by key and subKey couldn't be converted to int - returns an errors.InternalError.
func GetLastIndex(ctx context.Context, cacheService cache.Cache, key uuid.UUID, subKey cache.SubKey, errorTitle string) (int, error) {
	value, err := cacheService.GetValue(ctx, key, subKey)
	if err != nil {
		logger.Errorf("%s: GetLastIndex(): cache.GetValue: error: %s", key, err.Error())
		return 0, errors.NotFoundError(errorTitle, fmt.Sprintf("Error during getting cache by key: %s, subKey: %s", key.String(), string(subKey)))
	}
	intValue, converted := value.(int)
	if !converted {
		logger.Errorf("%s: couldn't convert value to int: %s", key, value)
		return 0, errors.InternalError(errorTitle, fmt.Sprintf("Value from cache couldn't be converted to int: %s", value))
	}
	return intValue, nil
}

// runCmdWithOutput runs command with keeping stdOut and stdErr
func runCmdWithOutput(cmd *exec.Cmd, stdOutput io.Writer, stdError *bytes.Buffer, successChannel chan bool, errorChannel chan error) {
	cmd.Stdout = stdOutput
	cmd.Stderr = stdError
	go func(cmd *exec.Cmd, successChannel chan bool, errChannel chan error) {
		err := cmd.Run()
		if err != nil {
			errChannel <- err
			successChannel <- false
		} else {
			successChannel <- true
		}
	}(cmd, successChannel, errorChannel)
}

// processStep processes each executor's step with cancel and timeout checks.
// If finishes by canceling, timeout or error - returns error.
// If finishes successfully returns nil.
func processStep(ctx context.Context, pipelineId uuid.UUID, cacheService cache.Cache, cancelChannel, successChannel chan bool, outDataBuffer, errorDataBuffer *bytes.Buffer, errorChannel chan error, errorCaseStatus, successCaseStatus pb.Status) error {
	select {
	case <-ctx.Done():
		finishByTimeout(ctx, pipelineId, cacheService)
		return fmt.Errorf("%s: context was done", pipelineId)
	case <-cancelChannel:
		processCancel(ctx, cacheService, pipelineId)
		return fmt.Errorf("%s: code processing was canceled", pipelineId)
	case ok := <-successChannel:
		var outData []byte = nil
		if outDataBuffer != nil {
			outData = outDataBuffer.Bytes()
		}
		if !ok {
			err := <-errorChannel
			var errorData []byte = nil
			if errorDataBuffer != nil {
				errorData = errorDataBuffer.Bytes()
			}
			processError(ctx, err, errorData, pipelineId, cacheService, errorCaseStatus)
			return fmt.Errorf("%s: code processing finishes with error: %s", pipelineId, err.Error())
		}
		processSuccess(ctx, outData, pipelineId, cacheService, successCaseStatus)
	}
	return nil
}

// cancelCheck checks cancel flag for code processing.
// If cancel flag doesn't exist in cache continue working.
// If context is done it means that code processing was finished (successfully/with error/timeout). Return.
// If cancel flag exists, and it is true it means that code processing was canceled. Set true to cancelChannel and return.
func cancelCheck(ctx context.Context, pipelineId uuid.UUID, cancelChannel chan bool, cacheService cache.Cache) {
	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			cancel, err := cacheService.GetValue(ctx, pipelineId, cache.Canceled)
			if err != nil {
				continue
			}
			if cancel.(bool) {
				cancelChannel <- true
			}
			return
		}
	}
}

// DeleteFolders removes all prepared folders for received LifeCycle
func DeleteFolders(pipelineId uuid.UUID, lc *fs_tool.LifeCycle) {
	logger.Infof("%s: DeleteFolders() ...\n", pipelineId)
	if err := lc.DeleteFolders(); err != nil {
		logger.Error("%s: DeleteFolders(): %s\n", pipelineId, err.Error())
	}
	logger.Infof("%s: DeleteFolders() complete\n", pipelineId)
	logger.Infof("%s: complete\n", pipelineId)
}

// finishByTimeout is used in case of runCode method finished by timeout
func finishByTimeout(ctx context.Context, pipelineId uuid.UUID, cacheService cache.Cache) {
	logger.Errorf("%s: code processing finishes because of timeout\n", pipelineId)

	// set to cache pipelineId: cache.SubKey_Status: Status_STATUS_RUN_TIMEOUT
	cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_RUN_TIMEOUT)
}

// processError processes error received during processing code via setting a corresponding status and output to cache
func processError(ctx context.Context, err error, data []byte, pipelineId uuid.UUID, cacheService cache.Cache, status pb.Status) {
	switch status {
	case pb.Status_STATUS_VALIDATION_ERROR:
		logger.Errorf("%s: Validate: %s\n", pipelineId, err.Error())

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_VALIDATION_ERROR)
	case pb.Status_STATUS_PREPARATION_ERROR:
		logger.Errorf("%s: Prepare: %s\n", pipelineId, err.Error())

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_PREPARATION_ERROR)
	case pb.Status_STATUS_COMPILE_ERROR:
		logger.Errorf("%s: Compile: err: %s, output: %s\n", pipelineId, err.Error(), data)

		cacheService.SetValue(ctx, pipelineId, cache.CompileOutput, "error: "+err.Error()+", output: "+string(data))

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_COMPILE_ERROR)
	case pb.Status_STATUS_RUN_ERROR:
		logger.Errorf("%s: Run: err: %s, output: %s\n", pipelineId, err.Error(), data)

		cacheService.SetValue(ctx, pipelineId, cache.RunError, "error: "+err.Error()+", output: "+string(data))

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_RUN_ERROR)
	}
}

// processSuccess processes case after successful code processing via setting a corresponding status and output to cache
func processSuccess(ctx context.Context, output []byte, pipelineId uuid.UUID, cacheService cache.Cache, status pb.Status) {
	switch status {
	case pb.Status_STATUS_PREPARING:
		logger.Infof("%s: Validate() finish\n", pipelineId)

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_PREPARING)
	case pb.Status_STATUS_COMPILING:
		logger.Infof("%s: Prepare() finish\n", pipelineId)

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_COMPILING)
	case pb.Status_STATUS_EXECUTING:
		logger.Infof("%s: Compile() finish\n", pipelineId)

		cacheService.SetValue(ctx, pipelineId, cache.CompileOutput, string(output))

		cacheService.SetValue(ctx, pipelineId, cache.RunOutput, "")

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_EXECUTING)
	case pb.Status_STATUS_FINISHED:
		logger.Infof("%s: Run() finish\n", pipelineId)

		cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_FINISHED)
	}
}

// processCancel process case when code processing was canceled
func processCancel(ctx context.Context, cacheService cache.Cache, pipelineId uuid.UUID) {
	logger.Infof("%s: was canceled\n", pipelineId)

	// set to cache pipelineId: cache.SubKey_Status: pb.Status_STATUS_CANCELED
	cacheService.SetValue(ctx, pipelineId, cache.Status, pb.Status_STATUS_CANCELED)
}
