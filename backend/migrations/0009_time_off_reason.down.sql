UPDATE buddy.request_types SET fields = jsonb_set(fields, '{reason,required}', 'false') WHERE code='time_off';
