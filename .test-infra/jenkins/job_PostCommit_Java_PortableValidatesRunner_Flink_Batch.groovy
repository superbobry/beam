/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import CommonJobProperties as commonJobProperties
import CommonTestProperties
import PostcommitJobBuilder

// This job runs the suite of ValidatesRunner tests against the Flink runner.
PostcommitJobBuilder.postCommitJob('beam_PostCommit_Java_PVR_Flink_Batch',
    'Run Java Flink PortableValidatesRunner Batch', 'Java Flink PortableValidatesRunner Batch Tests', this) {
      description('Runs the Java PortableValidatesRunner suite on the Flink runner.')

      // Set common parameters.
      commonJobProperties.setTopLevelMainJobProperties(delegate, defaultTimeout=240)

      // Publish all test results to Jenkins
      publishers {
        archiveJunit('**/build/test-results/**/*.xml')
      }

      // Gradle goals for this job.
      steps {
        gradle {
          rootBuildScriptDir(commonJobProperties.checkoutDir)
          tasks(":runners:flink:${CommonTestProperties.getFlinkVersion()}:job-server:validatesPortableRunnerBatch")
          commonJobProperties.setGradleSwitches(delegate)
        }
      }
    }
