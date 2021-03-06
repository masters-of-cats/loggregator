package agent_test

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/testservers"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/dropsonde/logs"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("Agent", func() {
	It("writes downstream via gRPC", func() {
		dopplerCleanup, dopplerPorts := testservers.StartRouter(
			testservers.BuildRouterConfig(0, 0),
		)
		defer dopplerCleanup()
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", dopplerPorts.GRPC),
		)
		defer agentCleanup()
		egressCleanup, egressClient := dopplerEgressClient(fmt.Sprintf("127.0.0.1:%d", dopplerPorts.GRPC))
		defer egressCleanup()

		var subscriptionClient plumbing.Doppler_SubscribeClient
		f := func() error {
			var err error
			subscriptionClient, err = egressClient.Subscribe(
				context.Background(),
				&plumbing.SubscriptionRequest{
					ShardID: "shard-id",
				},
			)
			return err
		}
		Eventually(f).ShouldNot(HaveOccurred())

		By("sending a message into agent")
		err := sendAppLog("test-app-id", "An event happened!", agentPorts.UDP)
		Expect(err).NotTo(HaveOccurred())

		By("reading a message from doppler")

		Eventually(func() string {
			resp, err := subscriptionClient.Recv()
			if err != nil {
				return ""
			}

			var e events.Envelope
			if err := proto.Unmarshal(resp.GetPayload(), &e); err != nil {
				return ""
			}

			return string(e.GetLogMessage().GetMessage())
		}, 3).Should(ContainSubstring("An event happened!"))
	})
})

func sendAppLog(appID, msg string, port int) error {
	dropsonde.Initialize(fmt.Sprintf("127.0.0.1:%d", port), "test-origin")
	return logs.SendAppLog(appID, msg, appID, "0")
}

func dopplerEgressClient(addr string) (func(), plumbing.DopplerClient) {
	creds, err := plumbing.NewClientCredentials(
		testservers.Cert("doppler.crt"),
		testservers.Cert("doppler.key"),
		testservers.Cert("loggregator-ca.crt"),
		"doppler",
	)
	Expect(err).ToNot(HaveOccurred())

	out, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	Expect(err).ToNot(HaveOccurred())
	return func() {
		_ = out.Close()
	}, plumbing.NewDopplerClient(out)
}
