/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha2"
	"sigs.k8s.io/kueue/pkg/util/testing"
	"sigs.k8s.io/kueue/pkg/workload"
	"sigs.k8s.io/kueue/test/e2e/framework"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = ginkgo.Describe("Kueue", func() {
	var ns *corev1.Namespace
	var sampleJob *batchv1.Job
	ginkgo.BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "e2e-",
			},
		}
		gomega.Expect(k8sClient.Create(ctx, ns)).To(gomega.Succeed())
		sampleJob = testing.MakeJob("test-job", ns.Name).Request("cpu", "1").Request("memory", "20Mi").
			Image("sleep", "gcr.io/k8s-staging-perf-tests/sleep:v0.0.3", []string{"5s"}).Obj()
		annotations := map[string]string{
			"kueue.x-k8s.io/queue-name": "main",
		}
		sampleJob.ObjectMeta.Annotations = annotations

		gomega.Expect(k8sClient.Create(ctx, sampleJob)).Should(gomega.Succeed())
	})
	ginkgo.AfterEach(func() {
		gomega.Expect(k8sClient.Delete(ctx, ns)).To(gomega.Succeed())
	})
	ginkgo.When("Creating a Job without a matching LocalQueue", func() {
		ginkgo.It("Should stay in suspended", func() {
			lookupKey := types.NamespacedName{Name: "test-job", Namespace: ns.Name}
			createdJob := &batchv1.Job{}
			gomega.Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, createdJob); err != nil {
					return false
				}
				return *createdJob.Spec.Suspend
			}, framework.Timeout, framework.Interval).Should(gomega.BeTrue())
			createdWorkload := &kueue.Workload{}
			gomega.Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, createdWorkload); err != nil {
					return false
				}
				return workload.InCondition(createdWorkload, kueue.WorkloadAdmitted)

			}, framework.Timeout, framework.Interval).Should(gomega.BeFalse())
			gomega.Expect(k8sClient.Delete(ctx, sampleJob)).Should(gomega.Succeed())
		})
	})
	ginkgo.When("Creating a Job With Queueing", func() {
		var (
			resourceKueue *kueue.ResourceFlavor
			localQueue    *kueue.LocalQueue
			clusterQueue  *kueue.ClusterQueue
		)
		ginkgo.BeforeEach(func() {
			resourceKueue = testing.MakeResourceFlavor("default").Obj()
			gomega.Expect(k8sClient.Create(ctx, resourceKueue)).Should(gomega.Succeed())
			localQueue = testing.MakeLocalQueue("main", ns.Name).Obj()
			clusterQueue = testing.MakeClusterQueue("cluster-queue").
				Resource(testing.MakeResource(corev1.ResourceCPU).
					Flavor(testing.MakeFlavor("default", "1").Obj()).Obj()).
				Resource(testing.MakeResource(corev1.ResourceMemory).
					Flavor(testing.MakeFlavor("default", "36Gi").Obj()).Obj()).Obj()
			localQueue.Spec.ClusterQueue = "cluster-queue"
			gomega.Expect(k8sClient.Create(ctx, clusterQueue)).Should(gomega.Succeed())
			gomega.Expect(k8sClient.Create(ctx, localQueue)).Should(gomega.Succeed())
		})
		ginkgo.AfterEach(func() {
			gomega.Expect(k8sClient.Delete(ctx, localQueue)).Should(gomega.Succeed())
			gomega.Expect(k8sClient.Delete(ctx, clusterQueue)).Should(gomega.Succeed())
			gomega.Expect(k8sClient.Delete(ctx, resourceKueue)).Should(gomega.Succeed())
		})
		ginkgo.It("Should unsuspend a job", func() {
			lookupKey := types.NamespacedName{Name: "test-job", Namespace: ns.Name}
			createdJob := &batchv1.Job{}
			createdWorkload := &kueue.Workload{}

			gomega.Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, createdJob); err != nil {
					return false
				}
				return !*createdJob.Spec.Suspend && createdJob.Status.Succeeded > 0
			}, framework.Timeout, framework.Interval).Should(gomega.BeTrue())
			gomega.Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, createdWorkload); err != nil {
					return false
				}
				return workload.InCondition(createdWorkload, kueue.WorkloadAdmitted) && workload.InCondition(createdWorkload, kueue.WorkloadFinished)

			}, framework.Timeout, framework.Interval).Should(gomega.BeTrue())
		})
	})
})