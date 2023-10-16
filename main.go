package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	pb "github.com/s5i/elections/proto"
	"google.golang.org/protobuf/proto"
)

var mandatesPerRegion = []int{12, 8, 14, 12, 13, 15, 12, 12, 10, 9, 12, 8, 14, 10, 9, 10, 9, 12, 20, 12, 12, 11, 15, 14, 12, 14, 9, 7, 9, 9, 12, 9, 16, 8, 10, 12, 9, 9, 10, 8, 12}

type mandateCount struct {
	party    string
	mandates int
}

func main() {
	resp, err := http.Get(`https://static2.wybory.gov.pl/sejmsenat2023/data/komitety.blob`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	p := &pb.Parties{}
	opts := proto.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(body, p); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	membership := map[int32]string{}
	name := map[int32]string{}
	for _, p := range p.Party {
		for _, cs := range p.Candidates {
			for _, c := range cs.Candidate {
				membership[c.Id] = p.Meta.Name
				name[c.Id] = c.Name
			}
		}
	}

	totals := map[string]int{}
	for i, numMandates := range mandatesPerRegion {
		fmt.Printf("Region %d:\n", i+1)
		regionMandates, err := getRegionMandates(i+1, numMandates, membership, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		sorted := []mandateCount{}
		for p, m := range regionMandates {
			sorted = append(sorted, mandateCount{p, m})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].mandates > sorted[j].mandates })
		for _, x := range sorted {
			fmt.Printf("\t%s: %d\n", x.party, x.mandates)
		}
		for p, m := range regionMandates {
			totals[p] += m
		}
	}

	fmt.Printf("Total:\n")
	sorted := []mandateCount{}
	for p, m := range totals {
		sorted = append(sorted, mandateCount{p, m})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].mandates > sorted[j].mandates })
	for _, x := range sorted {
		fmt.Printf("\t%s: %d\n", x.party, x.mandates)
	}
}

type weightedVote struct {
	party  string
	weight int
}

func getRegionMandates(id int, numMandates int, membership map[int32]string, name map[int32]string) (map[string]int, error) {
	resp, err := http.Get(fmt.Sprintf(`https://static.wybory.gov.pl/sejmsenat2023/data/obkw/okr_sejm/%d.blob`, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := &pb.Region{}
	opts := proto.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(body, r); err != nil {
		return nil, err
	}

	votes := map[string]int32{}

	for _, p := range r.WhoKnows[0].Person {
		votes[membership[p.Id]] += p.VoteCount.Cities
		votes[membership[p.Id]] += p.VoteCount.Villages
		votes[membership[p.Id]] += p.VoteCount.Ships
		votes[membership[p.Id]] += p.VoteCount.Foreign
	}

	weighted := []weightedVote{}
	for party, total := range votes {
		for i := 0; i < numMandates; i++ {
			weighted = append(weighted, weightedVote{party, int(total) / (i + 1)})
		}
	}

	sort.Slice(weighted, func(i, j int) bool { return weighted[i].weight > weighted[j].weight })

	mandates := map[string]int{}
	for i := 0; i < numMandates; i++ {
		mandates[weighted[i].party]++
	}
	return mandates, nil
}
