import part0 from './admin.json'
import part1 from './auth.json'
import part2 from './charts.json'
import part3 from './common.json'
import part4 from './dashboard.json'
import part5 from './doctor-queue.json'
import part6 from './home.json'
import part7 from './image-upload.json'
import part8 from './images.json'
import part9 from './navigation.json'
import part10 from './notifications.json'
import part11 from './reports.json'
import part12 from './review.json'
import part13 from './risk.json'
import part14 from './upload.json'
import part15 from './viewer.json'

const en = {
  ...part0,
  ...part1,
  ...part2,
  ...part3,
  ...part4,
  ...part5,
  ...part6,
  ...part7,
  ...part8,
  ...part9,
  ...part10,
  ...part11,
  ...part12,
  ...part13,
  ...part14,
  ...part15,
} as const

export default en
