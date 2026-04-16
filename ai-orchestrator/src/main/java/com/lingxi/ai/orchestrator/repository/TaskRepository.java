package com.lingxi.ai.orchestrator.repository;

import com.lingxi.ai.orchestrator.entity.Task;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface TaskRepository extends JpaRepository<Task, String> {
}
