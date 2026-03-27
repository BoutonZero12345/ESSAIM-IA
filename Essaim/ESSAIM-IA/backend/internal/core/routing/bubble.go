package routing

// bubble.go — This file is intentionally minimal.
// The Postier logic is implemented directly in the lifecycle/processor.go
// because it requires access to the LLM client, dispatcher, budget manager,
// and WebSocket hub — dependencies that would create circular imports if placed here.
//
// This package serves as documentation of the routing contract:
// - Workers send RPT_DONE to their PostierID (set at spawn time in processor.go)
// - The Processor routes packets to the correct handler based on the destination agent's role
// - When all workers in a bubble report done, the Processor's handlePostierReport method
//   triggers the Résumeur logic and escalates to the parent agent

// BubbleRoutingContract documents the data flow for the closed-group (Bulle) system.
// This is enforced in lifecycle/processor.go::HandlePacket.
//
// Data flow:
//  Parent (ARCHITECT/MANAGER)
//    └── SPAWN N Workers + 1 Postier (auto-created, if N >= 3)
//          ├── Worker 1 ──RPT_DONE──► Postier ──┐
//          ├── Worker 2 ──RPT_DONE──► Postier ──┤ (waits for all)
//          ├── Worker 3 ──RPT_DONE──► Postier ──┤
//          └── Worker N ──RPT_DONE──► Postier ──┘
//                                       │
//                                  Résumeur(s) ── if N >= 5
//                                       │
//                                  RPT_DONE ──► Parent
