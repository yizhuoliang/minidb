package graph

type Node struct {
	TxnNumber int
}

type Edge struct {
	From     *Node
	To       *Node
	EdgeType string
}

type Graph struct {
	Nodes []*Node
	Edges []*Edge
}

func NewGraph() *Graph {
	return &Graph{Nodes: make([]*Node, 0), Edges: make([]*Edge, 0)}
}

func (g *Graph) AddNode(txnNumber int) {
	newNode := &Node{TxnNumber: txnNumber}
	g.Nodes = append(g.Nodes, newNode)
}

func (g *Graph) AddEdge(from *Node, to *Node, edgeType string) {
	newEdge := &Edge{From: from, To: to, EdgeType: edgeType}
	g.Edges = append(g.Edges, newEdge)
}

func (g *Graph) GetNeighbors(node *Node) []*Node {
	neighbors := make([]*Node, 0)
	for _, edge := range g.Edges {
		if edge.From == node {
			neighbors = append(neighbors, edge.To)
		}
	}
	return neighbors
}

func (g *Graph) HasCycleWithConsecutiveRWEdges() bool {
	cycles := g.FindCycles()
	for _, cycle := range cycles {
		if len(cycle) == 1 {
			// this is nonsense, but still double check
			continue
		}
		prevIsRW := g.hasRWEdgeBetween(cycle[len(cycle)-1], cycle[0])

		for i := 0; i < len(cycle)-1; i++ {
			if g.hasRWEdgeBetween(cycle[i], cycle[i+1]) {
				if prevIsRW {
					return true
				} else {
					prevIsRW = true
				}
			}
		}
	}
	return false
}

func (g *Graph) hasRWEdgeBetween(x *Node, y *Node) bool {
	found := false
	for _, edge := range g.Edges {
		if edge.From == x && edge.To == y && edge.EdgeType == "rw" {
			found = true
		}
	}
	return found
}

func (g *Graph) FindCycles() [][]*Node {
	var cycles [][]*Node

	visited := make(map[*Node]bool)
	stack := []*Node{}

	g.dfsCyclesDetection(nil, visited, &stack, &cycles)

	return cycles
}

func (g *Graph) dfsCyclesDetection(node *Node, visited map[*Node]bool, stack *[]*Node, cycles *[][]*Node) {
	if node == nil {
		for _, startNode := range g.Nodes {
			if !visited[startNode] {
				g.dfsCyclesDetection(startNode, visited, stack, cycles)
			}
		}
		return
	}

	if visited[node] {
		// Check if node is in current stack (path)
		for i, stackNode := range *stack {
			if stackNode == node {
				*cycles = append(*cycles, append([]*Node(nil), (*stack)[i:]...))
				break
			}
		}
		return
	}

	visited[node] = true
	*stack = append(*stack, node)

	for _, neighbor := range g.GetNeighbors(node) {
		g.dfsCyclesDetection(neighbor, visited, stack, cycles)
	}

	*stack = (*stack)[:len(*stack)-1]
}

// func (g *Graph) HasCycleConsecutiveRWEdges() bool {
// 	visited := make(map[*Node]bool)
// 	inPath := make(map[*Node]bool)
// 	path := make([]*Node, 0)

// 	for _, node := range g.Nodes {
// 		if !visited[node] {
// 			if g.hasCycleConsecutiveRWEdgesRecursive(node, visited, inPath, path) {
// 				return true
// 			}
// 		}
// 	}
// 	return false
// }

// func (g *Graph) hasCycleConsecutiveRWEdgesRecursive(startNode *Node, visited map[*Node]bool, inPath map[*Node]bool, path []*Node) bool {
// 	visited[startNode] = true
// 	inPath[startNode] = true
// 	path = append(path, startNode)

// 	for _, neighbor := range g.GetNeighbors(startNode) {
// 		if !visited[neighbor] {
// 			if g.hasCycleConsecutiveRWEdgesRecursive(neighbor, visited, inPath, path) {
// 				return true
// 			}
// 		} else if inPath[neighbor] {
// 			// now we check if there is consecutive rw edges
// 			for
// 			return true
// 		}
// 	}

// 	inPath[startNode] = false
// 	return false
// }
