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
// Mocks generated by Mockito 5.0.16 from annotations
// in playground/test/pages/playground/states/example_selector_state_test.dart.
// Do not manually edit this file.

import 'dart:async' as _i6;

import 'package:mockito/mockito.dart' as _i1;
import 'package:playground/modules/editor/repository/code_repository/code_client/output_response.dart'
as _i4;
import 'package:playground/modules/examples/repositories/example_client/example_client.dart'
as _i5;
import 'package:playground/modules/examples/repositories/models/get_example_request.dart'
as _i8;
import 'package:playground/modules/examples/repositories/models/get_example_response.dart'
as _i3;
import 'package:playground/modules/examples/repositories/models/get_list_of_examples_request.dart'
as _i7;
import 'package:playground/modules/examples/repositories/models/get_list_of_examples_response.dart'
as _i2;

// ignore_for_file: avoid_redundant_argument_values
// ignore_for_file: avoid_setters_without_getters
// ignore_for_file: comment_references
// ignore_for_file: implementation_imports
// ignore_for_file: invalid_use_of_visible_for_testing_member
// ignore_for_file: prefer_const_constructors
// ignore_for_file: unnecessary_parenthesis
// ignore_for_file: camel_case_types

class _FakeGetListOfExampleResponse_0 extends _i1.Fake
    implements _i2.GetListOfExampleResponse {}

class _FakeGetExampleResponse_1 extends _i1.Fake
    implements _i3.GetExampleResponse {}

class _FakeOutputResponse_2 extends _i1.Fake implements _i4.OutputResponse {}

/// A class which mocks [ExampleClient].
///
/// See the documentation for Mockito's code generation for more information.
class MockExampleClient extends _i1.Mock implements _i5.ExampleClient {
  MockExampleClient() {
    _i1.throwOnMissingStub(this);
  }

  @override
  _i6.Future<_i2.GetListOfExampleResponse> getListOfExamples(_i7.GetListOfExamplesRequestWrapper? request) =>
      (super.noSuchMethod(Invocation.method(#getListOfExamples, [request]),
          returnValue: Future<_i2.GetListOfExampleResponse>.value(
              _FakeGetListOfExampleResponse_0()))
      as _i6.Future<_i2.GetListOfExampleResponse>);

  @override
  _i6.Future<_i3.GetExampleResponse> getExample(_i8.GetExampleRequestWrapper? request) =>
      (super.noSuchMethod(Invocation.method(#getExample, [request]),
          returnValue: Future<_i3.GetExampleResponse>.value(
              _FakeGetExampleResponse_1()))
      as _i6.Future<_i3.GetExampleResponse>);

  @override
  _i6.Future<_i4.OutputResponse> getExampleOutput(_i8.GetExampleRequestWrapper? request) =>
      (super.noSuchMethod(Invocation.method(#getExampleOutput, [request]),
          returnValue:
          Future<_i4.OutputResponse>.value(_FakeOutputResponse_2()))
      as _i6.Future<_i4.OutputResponse>);

  @override
  String toString() => super.toString();
}
