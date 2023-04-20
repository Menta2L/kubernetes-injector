package inject

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type injectionTestCase struct {
	description                         string
	annotatedPodTemplateSpecPath        string
	expectedInjectedPodTemplateSpecPath string
}

func TestEnvInjectInjection(t *testing.T) {
	var testCases = []injectionTestCase{
		{
			description:                         "Env",
			annotatedPodTemplateSpecPath:        "./testdata/env-annotated-pod.json",
			expectedInjectedPodTemplateSpecPath: "./testdata/env-mutated-pod.json",
		},
		{
			description:                         "Invalid Env",
			annotatedPodTemplateSpecPath:        "./testdata/invalid-env-annotated-pod.json",
			expectedInjectedPodTemplateSpecPath: "./testdata/invalid-env-mutated-pod.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			req, err := newTestAdmissionRequest(
				tc.annotatedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			expectedMod, err := os.ReadFile(
				tc.expectedInjectedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			mod, err := applyPatchToAdmissionRequest(req)
			if !assert.NoError(t, err) {
				return
			}

			assert.JSONEq(t, string(expectedMod), string(mod))
		})
	}
}
func TestMissingAnnotations(t *testing.T) {
	var testCases = []injectionTestCase{
		{
			description:                  "Missing",
			annotatedPodTemplateSpecPath: "./testdata/missing-annotations.json",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			req, err := newTestAdmissionRequest(
				tc.annotatedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}
			resp, err := sendAdmissionRequest(req)
			if !assert.NoError(t, err) {
				return
			}
			assert.True(t, resp.Allowed)
			assert.Empty(t, resp.Patch)
		})
	}
}
func TestSidecarInjection(t *testing.T) {
	var testCases = []injectionTestCase{
		{
			description:                         "Env",
			annotatedPodTemplateSpecPath:        "./testdata/sidecar-annotated-pod.json",
			expectedInjectedPodTemplateSpecPath: "./testdata/sidecar-mutated-pod.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			req, err := newTestAdmissionRequest(
				tc.annotatedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			expectedMod, err := os.ReadFile(
				tc.expectedInjectedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			mod, err := applyPatchToAdmissionRequest(req)
			if !assert.NoError(t, err) {
				return
			}
			assert.JSONEq(t, string(expectedMod), string(mod))
		})
	}
}
func TestBothInjection(t *testing.T) {
	var testCases = []injectionTestCase{
		{
			description:                         "Both",
			annotatedPodTemplateSpecPath:        "./testdata/mixed-annotated-pod.json",
			expectedInjectedPodTemplateSpecPath: "./testdata/mixed-mutated-pod.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			req, err := newTestAdmissionRequest(
				tc.annotatedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			expectedMod, err := os.ReadFile(
				tc.expectedInjectedPodTemplateSpecPath,
			)
			if !assert.NoError(t, err) {
				return
			}

			mod, err := applyPatchToAdmissionRequest(req)
			if !assert.NoError(t, err) {
				return
			}
			assert.JSONEq(t, string(expectedMod), string(mod))
		})
	}
}
