// A current observation, not a second map/location authority.
export type LocalMap = {
  place_id: string; place_name: string; world_time: string; observation_cursor: number
  reachable_places: { place_id: string; display_name: string; can_move_now: boolean; can_start_journey?: boolean; travel_minutes?: number }[]
  active_journey?: { journey_id: string; from_place_id: string; to_place_id: string; segment_place_id: string; scheduled_arrival_at: string }
  transit_works?: { from_place_id: string; to_place_id: string; ends_at: string }[]
}
export type MapMove = { from_place_id: string; to_place_id: string; expected_cursor: number; journey?: boolean }
