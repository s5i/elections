package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	pb "github.com/s5i/elections2023/proto"
	"google.golang.org/protobuf/proto"
)

var (
	printNames = flag.Bool("print_names", false, "Whether to print the names of candidates who got in.")
)

var aliases = map[string]string{
	"KOMITET WYBORCZY PRAWO I SPRAWIEDLIWOŚĆ":                                                            "PiS",
	"KOALICYJNY KOMITET WYBORCZY KOALICJA OBYWATELSKA PO .N IPL ZIELONI":                                 "KO",
	"KOALICYJNY KOMITET WYBORCZY TRZECIA DROGA POLSKA 2050 SZYMONA HOŁOWNI - POLSKIE STRONNICTWO LUDOWE": "TD/PSL",
	"KOMITET WYBORCZY NOWA LEWICA":                                                                       "Lewica",
	"KOMITET WYBORCZY KONFEDERACJA WOLNOŚĆ I NIEPODLEGŁOŚĆ":                                              "Konfederacja",
}

var mandatesPerRegion = []int{12, 8, 14, 12, 13, 15, 12, 12, 10, 9, 12, 8, 14, 10, 9, 10, 9, 12, 20, 12, 12, 11, 15, 14, 12, 14, 9, 7, 9, 9, 12, 9, 16, 8, 10, 12, 9, 9, 10, 8, 12}

type mandateCount struct {
	party    string
	mandates int
}

func main() {
	flag.Parse()

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
				if aliases[p.Meta.Name] != "" {
					membership[c.Id] = aliases[p.Meta.Name]
				}
				name[c.Id] = c.Name
			}
		}
	}

	totals := map[string]int{}
	for i, numMandates := range mandatesPerRegion {
		fmt.Printf("Region %d:\n", i+1)
		regionMandates, candidates, err := getRegionMandates(i+1, numMandates, membership, name)
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
			if *printNames {
				for _, c := range candidates[x.party] {
					fmt.Printf("\t\t%s\n", c)
				}
			}
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
	entity string
	weight int
}

func getRegionMandates(id int, numMandates int, membership map[int32]string, name map[int32]string) (map[string]int, map[string][]string, error) {
	resp, err := http.Get(fmt.Sprintf(`https://static.wybory.gov.pl/sejmsenat2023/data/obkw/okr_sejm/%d.blob`, id))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	r := &pb.Region{}
	opts := proto.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(body, r); err != nil {
		return nil, nil, err
	}

	votes := map[string]int32{}
	votesPerParty := map[string][]weightedVote{}

	for _, p := range r.WhoKnows[0].Person {
		v := int32(0)
		v += p.VoteCount.Cities
		v += p.VoteCount.Villages
		v += p.VoteCount.Ships
		v += p.VoteCount.Foreign

		votes[membership[p.Id]] += v
		votesPerParty[membership[p.Id]] = append(votesPerParty[membership[p.Id]], weightedVote{name[p.Id], int(v)})
	}
	for p := range votesPerParty {
		sort.Slice(votesPerParty[p], func(i, j int) bool { return votesPerParty[p][i].weight > votesPerParty[p][j].weight })
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
		mandates[weighted[i].entity]++
	}

	candidates := map[string][]string{}
	for p := range votesPerParty {
		for i := 0; i < mandates[p]; i++ {
			candidates[p] = append(candidates[p], votesPerParty[p][i].entity)
		}
	}
	return mandates, candidates, nil
}
