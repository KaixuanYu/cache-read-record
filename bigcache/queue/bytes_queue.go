package queue

import (
	"encoding/binary"
	"log"
	"time"
)

const (
	// Number of bytes to encode 0 in uvarint format 以uvarint格式编码为0的字节数
	minimumHeaderSize = 17 // 1 byte blobsize + timestampSizeInBytes + hashSizeInBytes
	// Bytes before left margin are not used. Zero index means element does not exist in queue, useful while reading slice from index
	leftMarginIndex = 1
)

var (
	errEmptyQueue       = &queueError{"Empty queue"}
	errInvalidIndex     = &queueError{"Index must be greater than zero. Invalid index."}
	errIndexOutOfBounds = &queueError{"Index out of range"}
)

// BytesQueue is a non-thread safe queue type of fifo based on bytes array.
// For every push operation index of entry is returned. It can be used to read the entry later
// BytesQueue是基于字节数组的fifo的非线程安全队列类型。
//对于每个推入操作，返回条目的索引。 以后可以用来阅读条目
type BytesQueue struct {
	full        bool   // 是否满了
	array       []byte // 字节数组
	capacity    int    // 目前容量（单位byte）
	maxCapacity int    // 最大容量
	head        int    // 头指针
	tail        int    // 尾指针
	count       int    // 应该是条目数量
	//rightMargin代表的是有数据的右边界
	// 1. 如果正常状态tail>head: leftMargin(1) -> head(30) -> tail,rightMargin(90) -> capacity(100)
	// rightMargin == tail
	// 2. 如果head > tail: leftMargin(1) -> tail(20) -> head(30) -> rightMargin(90) -> capacity(100)
	// 这是在1的基础上再push一个20的数据，因为capacity-tail=10<20，所以tail之后不能插入，但是head-leftMargin=29>20所以可以插入到head前。
	// 所以直接空着tail-capacity的10空间，直接在leftMargin往后开辟空间。
	rightMargin  int    // 有边界
	headerBuffer []byte // 这个会临时存要存入queue的data的长度，而且这个长度还是用varint编码的
	verbose      bool   //是否开启日志？
}

type queueError struct {
	message string
}

// getUvarintSize returns the number of bytes to encode x in uvarint format
// varint 是一个对整数的编码方式。proto buf 有用到这个东西
func getUvarintSize(x uint32) int {
	if x < 128 {
		return 1
	} else if x < 16384 {
		return 2
	} else if x < 2097152 {
		return 3
	} else if x < 268435456 {
		return 4
	} else {
		return 5
	}
}

// NewBytesQueue initialize new bytes queue. 初始化一个新的字节队列。
// capacity is used in bytes array allocation 参数capacity用来分配内存
// When verbose flag is set then information about memory allocation are printed verbose设置后，内存分配会被输出
func NewBytesQueue(capacity int, maxCapacity int, verbose bool) *BytesQueue {
	return &BytesQueue{
		array:        make([]byte, capacity),
		capacity:     capacity,
		maxCapacity:  maxCapacity,
		headerBuffer: make([]byte, binary.MaxVarintLen32), //5字节长度？
		tail:         leftMarginIndex,
		head:         leftMarginIndex,
		rightMargin:  leftMarginIndex,
		verbose:      verbose,
	}
}

// Reset removes all entries from queue 重置
func (q *BytesQueue) Reset() {
	// Just reset indexes
	q.tail = leftMarginIndex
	q.head = leftMarginIndex
	q.rightMargin = leftMarginIndex
	q.count = 0
	q.full = false
}

// Push copies entry at the end of queue and moves tail pointer. Allocates more space if needed.
// Returns index for pushed data or error if maximum size queue limit is reached.
// Push 拷贝entry到queue的尾部，然后移动tail指针。如果需要，会分配更多空间。
// 返回存入之后的数据的index，或者如果超出限制就返回error
func (q *BytesQueue) Push(data []byte) (int, error) {
	dataLen := len(data)
	headerEntrySize := getUvarintSize(uint32(dataLen)) //对数据的长度进行了varint编码

	if !q.canInsertAfterTail(dataLen + headerEntrySize) {
		if q.canInsertBeforeHead(dataLen + headerEntrySize) {
			//这个if就是判断的leftMarginIndex -> head 的空间够不够。因为当head>tail的时候，canInsertAfterTail判断了
			q.tail = leftMarginIndex
		} else if q.capacity+headerEntrySize+dataLen >= q.maxCapacity && q.maxCapacity > 0 {
			//如果装的最大数据已经超过 maxCapacity 就直接返回full queue
			return -1, &queueError{"Full queue. Maximum size limit reached."}
		} else {
			//否则就扩充内存
			q.allocateAdditionalMemory(dataLen + headerEntrySize)
		}
	}

	index := q.tail

	q.push(data, dataLen)

	return index, nil
}

func (q *BytesQueue) allocateAdditionalMemory(minimum int) {
	start := time.Now()
	if q.capacity < minimum {
		q.capacity += minimum //如果capacity本身比需求的内存少，就先加上需求的内存的容量（这样可以避免扩容后仍然不满足要求）
	}
	q.capacity = q.capacity * 2 //2倍扩容
	if q.capacity > q.maxCapacity && q.maxCapacity > 0 {
		q.capacity = q.maxCapacity //超过限制只给最大限制的容量
	}

	oldArray := q.array
	q.array = make([]byte, q.capacity) //重新开辟空间

	if leftMarginIndex != q.rightMargin { //这代表queue不空
		copy(q.array, oldArray[:q.rightMargin])

		if q.tail <= q.head {
			if q.tail != q.head { //不满也不空
				headerEntrySize := getUvarintSize(uint32(q.head - q.tail))
				emptyBlobLen := q.head - q.tail - headerEntrySize
				//todo 这里push的count++后是不是就多加了1了？而且临界值好像不准啊。
				//如果长度正好是128，那么在这么计算之后，填充的是127？
				// 我认为这是一个bug，并给作者提了issue ： https://github.com/allegro/bigcache/issues/253
				// 在v2.2.1 - v3.0.0 (我阅读代码时候的版本) 都有这个bug。而且我认为修复起来很麻烦。
				// 如果没有后续版本，最好用 v2.2.0 以及之前版本

				// 为啥要push一个空byte到free space，因为head和tail要被重置。head会被重置到最前面
				// 如果我们之后有pop操作，会从head pop一个元素，这个一个元素就是varintSize+bug，空的那一块没有varint，所以解析不出来
				// 然后这里push操作就是将这块空space，varint一下，符合编码~
				q.push(make([]byte, emptyBlobLen), emptyBlobLen) //清空中间空出来那块tail->head
			}

			q.head = leftMarginIndex
			q.tail = q.rightMargin
		}
	}

	q.full = false

	if q.verbose {
		log.Printf("Allocated new queue in %s; Capacity: %d \n", time.Since(start), q.capacity)
	}
}

func (q *BytesQueue) push(data []byte, len int) {
	//将数据长度的varint编码放进 q.headerBuffer 中。并获取到 q.headerBuffer 的长度
	headerEntrySize := binary.PutUvarint(q.headerBuffer, uint64(len))
	//将header放进queue中
	q.copy(q.headerBuffer, headerEntrySize)
	//将data放进queue中
	q.copy(data, len)

	if q.tail > q.head {
		q.rightMargin = q.tail
	}
	if q.tail == q.head { //队列头和队列尾重合，队列满了。
		q.full = true
	}

	q.count++ //放进一个元素，count+1
}

//将数据放入 BytesQueue.array 中，并移动tail到队尾
func (q *BytesQueue) copy(data []byte, len int) {
	q.tail += copy(q.array[q.tail:], data[:len])
}

// Pop reads the oldest entry from queue and moves head pointer to the next one
// Pop 读取最旧的entry，并且将head指针移动到下一个位置。
func (q *BytesQueue) Pop() ([]byte, error) {
	data, headerEntrySize, err := q.peek(q.head)
	if err != nil {
		return nil, err
	}
	size := len(data)

	q.head += headerEntrySize + size
	q.count--

	if q.head == q.rightMargin {
		q.head = leftMarginIndex
		if q.tail == q.rightMargin {
			q.tail = leftMarginIndex
		}
		q.rightMargin = q.tail
	}

	q.full = false

	return data, nil
}

// Peek reads the oldest entry from list without moving head pointer
// Peek读取最老的entry但是不移动head pointer
func (q *BytesQueue) Peek() ([]byte, error) {
	data, _, err := q.peek(q.head)
	return data, err
}

// Get reads entry from index 从index读取一个entry
func (q *BytesQueue) Get(index int) ([]byte, error) {
	data, _, err := q.peek(index)
	return data, err
}

// CheckGet checks if an entry can be read from index
// 检查是否可以从某个index读取到数据
func (q *BytesQueue) CheckGet(index int) error {
	return q.peekCheckErr(index)
}

// Capacity returns number of allocated bytes for queue
func (q *BytesQueue) Capacity() int {
	return q.capacity
}

// Len returns number of entries kept in queue
func (q *BytesQueue) Len() int {
	return q.count
}

// Error returns error message
func (e *queueError) Error() string {
	return e.message
}

// peekCheckErr is identical to peek, but does not actually return any data
//peekCheckErr与peek相同，但实际上不返回任何数据
func (q *BytesQueue) peekCheckErr(index int) error {

	if q.count == 0 {
		return errEmptyQueue
	}

	if index <= 0 {
		return errInvalidIndex
	}

	if index >= len(q.array) {
		return errIndexOutOfBounds
	}
	return nil
}

// peek returns the data from index and the number of bytes to encode the length of the data in uvarint format
func (q *BytesQueue) peek(index int) ([]byte, int, error) {
	err := q.peekCheckErr(index)
	if err != nil {
		return nil, 0, err
	}

	blockSize, n := binary.Uvarint(q.array[index:])
	return q.array[index+n : index+n+int(blockSize)], n, nil
}

// canInsertAfterTail returns true if it's possible to insert an entry of size of need after the tail of the queue
// 判断是否可以在尾后插入。
func (q *BytesQueue) canInsertAfterTail(need int) bool {
	if q.full { //队列满，肯定不可以
		return false
	}
	if q.tail >= q.head { //如果目前是 队列头（0）-> head(3) -> tail(104) -> capacity(512) ，那么就看 512-104是不是够用
		return q.capacity-q.tail >= need
	}
	// 1. there is exactly need bytes between head and tail, so we do not need
	// to reserve extra space for a potential empty entry when realloc this queue
	// 1.头和尾之间确实需要字节，因此在重新分配此队列时，我们不需要为潜在的空条目保留额外的空间
	// 2. still have unused space between tail and head, then we must reserve
	// at least headerEntrySize bytes so we can put an empty entry
	// 2.头和尾之间仍然有未使用的空间，那么我们必须至少保留headerEntrySize字节，以便我们可以放置一个空条目
	// 现在是队列头（0）-> tail(3) -> head(104) -> capacity(512)，tail-head是空区域，就判断104-3是不是大于我想要的空间
	// todo 为啥要留一个空条目呢？这个空条目就是用来在执行 allocateAdditionalMemory 填充的。
	return q.head-q.tail == need || q.head-q.tail >= need+minimumHeaderSize
}

// canInsertBeforeHead returns true if it's possible to insert an entry of size of need before the head of the queue
// 是否可以插入到队头之前？
func (q *BytesQueue) canInsertBeforeHead(need int) bool {
	if q.full { //队列满了不可以
		return false
	}
	if q.tail >= q.head { // 如果目前是 队列头（0）-> leftMarginIndex(1) -> head(30) -> tail(104) -> capacity(512)
		// 那么看 30-10是否>need+minimumHeaderSize
		return q.head-leftMarginIndex == need || q.head-leftMarginIndex >= need+minimumHeaderSize
	}
	return q.head-q.tail == need || q.head-q.tail >= need+minimumHeaderSize
}
