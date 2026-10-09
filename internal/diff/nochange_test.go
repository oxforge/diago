package diff

// noChange reports whether d records no added, removed or changed node,
// edge or group.
func noChange(d GraphDiff) bool {
	return len(d.AddedNodes)+len(d.RemovedNodes)+len(d.ChangedNodes)+
		len(d.AddedEdges)+len(d.RemovedEdges)+len(d.ChangedEdges)+
		len(d.AddedGroups)+len(d.RemovedGroups)+len(d.ChangedGroups) == 0
}
